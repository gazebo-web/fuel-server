package main

import (
	"archive/tar"
	"bytes"
	"container/list"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// gzmeshVersion is part of the cache key; bump it with gzmesh/run.py VERSION.
const gzmeshVersion = "0.2.0"

type config struct {
	Addr, Token             string
	Python, GzmeshDir, Jail string
	WorkDir                 string
	Concurrency, QueueLen   int
	JobTimeout              time.Duration // wall time per job, then the process group is killed
	MaxWait, DefaultWait    time.Duration // how long POST /v1/jobs blocks before answering 202
	KeepResults             time.Duration // how long finished async results stay available
	CacheBytes, MaxOutput   int64
	MaxJobBytes             int64
	MemoryBytes             int64 // RLIMIT_AS inside the jail
	CPUSeconds              int   // RLIMIT_CPU inside the jail
	// ExtraRO are paths the jail binds read-only over its empty /home and
	// /root: a development venv and its interpreter. The image needs none.
	ExtraRO []string
	command func(dir string) *exec.Cmd
}

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// result is a finished job: the tarball, and whether the pipeline itself
// failed (as opposed to the model code, which report.json describes).
type result struct {
	tar      []byte
	internal bool
	err      error
}

type job struct {
	id   string
	key  string
	done chan struct{}
	res  result
	at   time.Time
}

type worker struct {
	cfg   config
	slots chan struct{} // running jobs
	queue chan struct{} // running + waiting jobs

	mu      sync.Mutex
	jobs    map[string]*job // by id, until KeepResults after they finish
	running map[string]*job // by cache key, so identical jobs share one run
	cache   *lru
}

func newWorker(cfg config) (*worker, error) {
	switch cfg.Jail {
	case "bwrap", "none":
	default:
		return nil, fmt.Errorf("MESHWORKER_JAIL %q: want bwrap or none", cfg.Jail)
	}
	if err := os.MkdirAll(cfg.WorkDir, 0o700); err != nil {
		return nil, err
	}
	w := &worker{
		cfg:     cfg,
		slots:   make(chan struct{}, cfg.Concurrency),
		queue:   make(chan struct{}, cfg.Concurrency+cfg.QueueLen),
		jobs:    map[string]*job{},
		running: map[string]*job{},
		cache:   newLRU(cfg.CacheBytes),
	}
	if w.cfg.command == nil {
		w.cfg.command = w.jailed
	}
	return w, nil
}

func (w *worker) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(rw http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(rw, "ok")
	})
	mux.HandleFunc("/v1/jobs", w.auth(w.handleSubmit))
	mux.HandleFunc("/v1/jobs/", w.auth(w.handleGet))
	return mux
}

func (w *worker) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if w.cfg.Token != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(w.cfg.Token)) != 1 {
				httpError(rw, http.StatusUnauthorized, "missing or wrong token")
				return
			}
		}
		h(rw, r)
	}
}

func httpError(rw http.ResponseWriter, status int, msg string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(map[string]string{"error": msg})
}

// jobKey canonicalizes the job (re-encoding sorts the keys) and hashes it
// with the pipeline version, so identical requests share a cached result.
func jobKey(fields map[string]any) string {
	b, _ := json.Marshal(fields)
	sum := sha256.Sum256(append([]byte(gzmeshVersion+"\n"), b...))
	return hex.EncodeToString(sum[:])
}

func (w *worker) handleSubmit(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(rw, http.StatusMethodNotAllowed, "POST a job")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, w.cfg.MaxJobBytes+1))
	if err != nil {
		httpError(rw, http.StatusBadRequest, err.Error())
		return
	}
	if int64(len(body)) > w.cfg.MaxJobBytes {
		httpError(rw, http.StatusRequestEntityTooLarge, fmt.Sprintf("job is larger than %d bytes", w.cfg.MaxJobBytes))
		return
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		httpError(rw, http.StatusBadRequest, "job must be a JSON object: "+err.Error())
		return
	}
	if name, _ := fields["name"].(string); !nameRE.MatchString(name) {
		httpError(rw, http.StatusBadRequest, "name must be lowercase letters, digits and underscores, starting with a letter")
		return
	}
	if code, _ := fields["code"].(string); strings.TrimSpace(code) == "" {
		httpError(rw, http.StatusBadRequest, "code is required")
		return
	}
	wait := w.cfg.DefaultWait
	if s := r.URL.Query().Get("wait"); s != "" {
		secs, err := strconv.Atoi(s)
		if err != nil || secs < 0 {
			httpError(rw, http.StatusBadRequest, "wait must be seconds")
			return
		}
		wait = time.Duration(secs) * time.Second
		if wait > w.cfg.MaxWait {
			wait = w.cfg.MaxWait
		}
	}

	key := jobKey(fields)
	if tarball, ok := w.cache.get(key); ok {
		rw.Header().Set("X-Cache", "hit")
		writeTar(rw, result{tar: tarball})
		return
	}
	j, err := w.submit(key, body)
	if err != nil {
		rw.Header().Set("Retry-After", "10")
		httpError(rw, http.StatusTooManyRequests, err.Error())
		return
	}
	select {
	case <-j.done:
		writeTar(rw, j.res)
	case <-time.After(wait):
		rw.Header().Set("Location", "/v1/jobs/"+j.id)
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(rw).Encode(map[string]string{"job_id": j.id, "status": "running"})
	case <-r.Context().Done():
	}
}

func (w *worker) handleGet(rw http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	w.mu.Lock()
	j := w.jobs[id]
	w.mu.Unlock()
	if j == nil {
		httpError(rw, http.StatusNotFound, "no such job (results are kept for "+w.cfg.KeepResults.String()+")")
		return
	}
	select {
	case <-j.done:
		writeTar(rw, j.res)
	default:
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(rw).Encode(map[string]string{"job_id": j.id, "status": "running"})
	}
}

func writeTar(rw http.ResponseWriter, res result) {
	switch {
	case res.err != nil:
		httpError(rw, http.StatusInternalServerError, res.err.Error())
	default:
		rw.Header().Set("Content-Type", "application/x-tar")
		if res.internal {
			rw.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = rw.Write(res.tar)
	}
}

// submit starts a job, or joins an identical one already running. It
// fails when the queue is full.
func (w *worker) submit(key string, body []byte) (*job, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.expireLocked()
	if j, ok := w.running[key]; ok {
		return j, nil
	}
	select {
	case w.queue <- struct{}{}:
	default:
		return nil, errors.New("the mesh worker is busy; retry shortly")
	}
	j := &job{id: newID(), key: key, done: make(chan struct{})}
	w.jobs[j.id] = j
	w.running[key] = j
	go func() {
		defer func() { <-w.queue }()
		w.slots <- struct{}{}
		res := w.execute(body)
		<-w.slots
		w.mu.Lock()
		j.res, j.at = res, time.Now()
		delete(w.running, key)
		w.mu.Unlock()
		if res.err == nil && !res.internal {
			w.cache.put(key, res.tar)
		}
		close(j.done)
	}()
	return j, nil
}

func (w *worker) expireLocked() {
	for id, j := range w.jobs {
		select {
		case <-j.done:
			if time.Since(j.at) > w.cfg.KeepResults {
				delete(w.jobs, id)
			}
		default:
		}
	}
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// execute runs one job in a fresh directory and tars what it wrote.
func (w *worker) execute(body []byte) result {
	dir, err := os.MkdirTemp(w.cfg.WorkDir, "job-")
	if err != nil {
		return result{err: err}
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "job.json"), body, 0o644); err != nil {
		return result{err: err}
	}
	if err := os.Mkdir(filepath.Join(dir, "out"), 0o777); err != nil {
		return result{err: err}
	}

	ctx, cancel := context.WithTimeout(context.Background(), w.cfg.JobTimeout)
	defer cancel()
	cmd := w.cfg.command(dir)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &limitedBuffer{buf: &stderr, max: 64 << 10}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return result{err: fmt.Errorf("start job: %w", err)}
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	select {
	case err = <-waitErr:
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-waitErr
		return result{err: fmt.Errorf("the job ran longer than %s and was stopped", w.cfg.JobTimeout)}
	}
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		return result{err: err}
	}
	log.Printf("meshworker: job %s exit %d in %s", filepath.Base(dir), code, time.Since(start).Round(time.Millisecond))

	out := filepath.Join(dir, "out")
	if _, err := os.Stat(filepath.Join(out, "report.json")); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if code == -1 || strings.Contains(msg, "MemoryError") {
			msg = "the job ran out of memory or CPU time; " + msg
		}
		return result{err: fmt.Errorf("the job exited (%d) without a report: %s", code, lastLines(msg, 5))}
	}
	tarball, err := tarDir(out, w.cfg.MaxOutput)
	if err != nil {
		return result{err: err}
	}
	return result{tar: tarball, internal: code == 1}
}

// jailed is the per-job sandbox (plan §5.7): bubblewrap with no network
// and fresh namespaces, a read-only view of the image with /home, /root,
// /tmp and the other jobs hidden, only this job's directory writable, and
// rlimits via prlimit. Hiding matters even without a network: anything the
// code can read it could copy into the model files it returns. The worker
// container itself runs under gVisor in production.
func (w *worker) jailed(dir string) *exec.Cmd {
	// The job directory appears at /tmp/job: the root is read-only, so the
	// mount point has to be inside the jail's own /tmp.
	py := []string{w.cfg.Python, "-m", "gzmesh.run", "/tmp/job/job.json", "/tmp/job/out"}
	limits := []string{"prlimit",
		"--as=" + strconv.FormatInt(w.cfg.MemoryBytes, 10),
		"--cpu=" + strconv.Itoa(w.cfg.CPUSeconds),
		"--fsize=" + strconv.FormatInt(w.cfg.MaxOutput*2, 10),
		"--core=0", "--"}
	if w.cfg.Jail == "none" {
		py[3], py[4] = filepath.Join(dir, "job.json"), filepath.Join(dir, "out")
		cmd := exec.Command(py[0], py[1:]...)
		cmd.Env = jobEnv(w.cfg.GzmeshDir, os.TempDir())
		return cmd
	}
	args := []string{
		"--unshare-all", "--die-with-parent", "--new-session",
		"--ro-bind", "/", "/",
		"--proc", "/proc", "--dev", "/dev",
		"--tmpfs", "/tmp", "--tmpfs", w.cfg.WorkDir, "--tmpfs", "/root", "--tmpfs", "/home",
	}
	for _, p := range w.cfg.ExtraRO {
		args = append(args, "--ro-bind", p, p)
	}
	args = append(args,
		"--bind", dir, "/tmp/job",
		"--chdir", "/tmp/job",
		"--clearenv",
	)
	for _, kv := range jobEnv(w.cfg.GzmeshDir, "/tmp") {
		k, v, _ := strings.Cut(kv, "=")
		args = append(args, "--setenv", k, v)
	}
	args = append(args, limits...)
	args = append(args, py...)
	return exec.Command("bwrap", args...)
}

func jobEnv(gzmeshDir, tmp string) []string {
	return []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"PYTHONPATH=" + gzmeshDir,
		"PYTHONDONTWRITEBYTECODE=1",
		"HOME=" + tmp,
		"TMPDIR=" + tmp,
		"MPLCONFIGDIR=" + tmp,
		"OMP_NUM_THREADS=2", "OPENBLAS_NUM_THREADS=2", "MKL_NUM_THREADS=2",
	}
}

// tarDir archives dir (report.json first), refusing more than max bytes.
func tarDir(dir string, max int64) ([]byte, error) {
	var files []string
	var total int64
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("unexpected non-regular file %s", d.Name())
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if total > max {
			return fmt.Errorf("the model is larger than %d MB", max>>20)
		}
		rel, _ := filepath.Rel(dir, p)
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool {
		if (files[i] == "report.json") != (files[j] == "report.json") {
			return files[i] == "report.json"
		}
		return files[i] < files[j]
	})
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return nil, err
		}
		if err := tw.WriteHeader(&tar.Header{Name: rel, Mode: 0o644, Size: int64(len(b)), ModTime: time.Unix(0, 0)}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(b); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.max - l.buf.Len(); room > 0 {
		if len(p) > room {
			l.buf.Write(p[:room])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// lru caches finished tarballs by job key, up to a byte budget.
type lru struct {
	mu    sync.Mutex
	max   int64
	size  int64
	order *list.List
	items map[string]*list.Element
}

type lruEntry struct {
	key string
	val []byte
}

func newLRU(max int64) *lru {
	return &lru{max: max, order: list.New(), items: map[string]*list.Element{}}
}

func (c *lru) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.order.MoveToFront(e)
		return e.Value.(*lruEntry).val, true
	}
	return nil, false
}

func (c *lru) put(key string, val []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if int64(len(val)) > c.max {
		return
	}
	if e, ok := c.items[key]; ok {
		c.size -= int64(len(e.Value.(*lruEntry).val))
		c.order.Remove(e)
	}
	c.items[key] = c.order.PushFront(&lruEntry{key, val})
	c.size += int64(len(val))
	for c.size > c.max {
		e := c.order.Back()
		ent := e.Value.(*lruEntry)
		c.order.Remove(e)
		delete(c.items, ent.key)
		c.size -= int64(len(ent.val))
	}
}
