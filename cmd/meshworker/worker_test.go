package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// stub stands in for "python -m gzmesh.run JOB OUT". The job's code
// selects a behaviour; every run appends to runs.log.
const stub = `#!/bin/sh
job="$3"; out="$4"
echo run >> "$(dirname "$0")/runs.log"
case "$(cat "$job")" in
  *SLEEP*) sleep 3 ;;
esac
case "$(cat "$job")" in
  *INTERNAL*) echo '{"ok":false,"error":"internal error: boom"}' > "$out/report.json"; exit 1 ;;
  *CODEERR*) echo '{"ok":false,"error":"ZeroDivisionError","line":4}' > "$out/report.json"; exit 2 ;;
  *NOREPORT*) echo "Traceback: MemoryError" >&2; exit 1 ;;
  *BIG*) head -c 2000000 /dev/zero > "$out/big.bin" ;;
esac
mkdir -p "$out/models/thing/meshes"
echo glb > "$out/models/thing/meshes/thing.glb"
echo png > "$out/preview.png"
echo '{"ok":true,"name":"thing"}' > "$out/report.json"
`

func newTestWorker(t *testing.T, mod func(*config)) (*worker, *httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	py := filepath.Join(dir, "python")
	if err := os.WriteFile(py, []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Jail, cfg.Python, cfg.WorkDir = "none", py, filepath.Join(dir, "work")
	cfg.Concurrency, cfg.QueueLen = 2, 2
	cfg.DefaultWait = 5 * time.Second
	cfg.MaxOutput = 1 << 20
	if mod != nil {
		mod(&cfg)
	}
	w, err := newWorker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(w.routes())
	t.Cleanup(srv.Close)
	return w, srv, dir
}

func post(t *testing.T, url, body string, header ...string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func jobJSON(code string) string {
	b, _ := json.Marshal(map[string]any{"name": "thing", "code": code})
	return string(b)
}

func untar(t *testing.T, r io.Reader) (names []string, files map[string]string) {
	t.Helper()
	files = map[string]string{}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return names, files
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		names = append(names, h.Name)
		files[h.Name] = string(b)
	}
}

func runs(t *testing.T, dir string) int {
	b, _ := os.ReadFile(filepath.Join(dir, "runs.log"))
	return strings.Count(string(b), "run")
}

func TestJobSuccessAndCache(t *testing.T) {
	_, srv, dir := newTestWorker(t, nil)
	resp := post(t, srv.URL+"/v1/jobs", jobJSON("def build(): ..."))
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/x-tar" {
		t.Fatalf("status %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	names, files := untar(t, resp.Body)
	if names[0] != "report.json" || !strings.Contains(files["report.json"], `"ok":true`) {
		t.Fatalf("report first: %v", names)
	}
	if files["models/thing/meshes/thing.glb"] != "glb\n" || files["preview.png"] == "" {
		t.Fatalf("files %v", names)
	}

	// The same job again comes from the cache; whitespace in the JSON
	// doesn't change the key.
	resp = post(t, srv.URL+"/v1/jobs", `{"code": "def build(): ...",   "name": "thing"}`)
	if resp.StatusCode != 200 || resp.Header.Get("X-Cache") != "hit" {
		t.Fatalf("want cache hit: %d %q", resp.StatusCode, resp.Header.Get("X-Cache"))
	}
	if n := runs(t, dir); n != 1 {
		t.Fatalf("ran %d times", n)
	}
}

func TestJobFailures(t *testing.T) {
	_, srv, _ := newTestWorker(t, func(c *config) { c.JobTimeout = 500 * time.Millisecond })

	// The model code failed: 200, and the report says why.
	resp := post(t, srv.URL+"/v1/jobs", jobJSON("CODEERR"))
	_, files := untar(t, resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(files["report.json"], `"line":4`) {
		t.Fatalf("code error: %d %v", resp.StatusCode, files)
	}

	// The pipeline failed: 500 with the report.
	resp = post(t, srv.URL+"/v1/jobs", jobJSON("INTERNAL"))
	_, files = untar(t, resp.Body)
	if resp.StatusCode != 500 || !strings.Contains(files["report.json"], "internal error") {
		t.Fatalf("internal: %d %v", resp.StatusCode, files)
	}

	for code, want := range map[string]string{
		"SLEEP":    "ran longer than 500ms",
		"NOREPORT": "out of memory or CPU time",
	} {
		resp = post(t, srv.URL+"/v1/jobs", jobJSON(code))
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 500 || !strings.Contains(string(b), want) {
			t.Errorf("%s: %d %s", code, resp.StatusCode, b)
		}
	}
}

func TestOutputLimit(t *testing.T) {
	_, srv, _ := newTestWorker(t, nil)
	resp := post(t, srv.URL+"/v1/jobs", jobJSON("BIG"))
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 500 || !strings.Contains(string(b), "larger than 1 MB") {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
}

func TestAsyncAndQueue(t *testing.T) {
	_, srv, _ := newTestWorker(t, func(c *config) { c.Concurrency, c.QueueLen = 1, 0 })

	resp := post(t, srv.URL+"/v1/jobs?wait=0", jobJSON("SLEEP one"))
	var acc struct {
		JobID string `json:"job_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&acc)
	if resp.StatusCode != 202 || acc.JobID == "" || resp.Header.Get("Location") != "/v1/jobs/"+acc.JobID {
		t.Fatalf("want 202 with a job id: %d %+v", resp.StatusCode, acc)
	}

	// The only slot is taken and there is no queue: a different job is refused.
	resp = post(t, srv.URL+"/v1/jobs?wait=0", jobJSON("SLEEP two"))
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("want 429: %d", resp.StatusCode)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(srv.URL + "/v1/jobs/" + acc.JobID)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode == 200 {
			names, _ := untar(t, resp.Body)
			if names[0] != "report.json" {
				t.Fatalf("%v", names)
			}
			break
		}
		if resp.StatusCode != 202 || time.Now().After(deadline) {
			t.Fatalf("poll: %d", resp.StatusCode)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if resp, _ := http.Get(srv.URL + "/v1/jobs/nope"); resp.StatusCode != 404 {
		t.Fatalf("unknown job: %d", resp.StatusCode)
	}
}

func TestIdenticalJobsShareARun(t *testing.T) {
	_, srv, dir := newTestWorker(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := post(t, srv.URL+"/v1/jobs", jobJSON("SLEEP same"))
			if resp.StatusCode != 200 {
				t.Errorf("status %d", resp.StatusCode)
			}
		}()
	}
	wg.Wait()
	if n := runs(t, dir); n != 1 {
		t.Fatalf("ran %d times", n)
	}
}

func TestRequestChecks(t *testing.T) {
	_, srv, _ := newTestWorker(t, func(c *config) { c.Token = "s3cret"; c.MaxJobBytes = 1000 })
	auth := []string{"Authorization", "Bearer s3cret"}
	for _, tc := range []struct {
		body   string
		header []string
		status int
	}{
		{jobJSON("x"), nil, 401},
		{jobJSON("x"), []string{"Authorization", "Bearer nope"}, 401},
		{`{"name":"Bad Name","code":"x"}`, auth, 400},
		{`{"name":"thing"}`, auth, 400},
		{`not json`, auth, 400},
		{jobJSON(strings.Repeat("x", 2000)), auth, 413},
		{jobJSON("x"), auth, 200},
	} {
		resp := post(t, srv.URL+"/v1/jobs", tc.body, tc.header...)
		if resp.StatusCode != tc.status {
			b, _ := io.ReadAll(resp.Body)
			t.Errorf("%.40s: got %d want %d (%s)", tc.body, resp.StatusCode, tc.status, b)
		}
	}
	if resp, _ := http.Get(srv.URL + "/healthz"); resp.StatusCode != 200 {
		t.Errorf("healthz %d", resp.StatusCode)
	}
}

func TestLRU(t *testing.T) {
	c := newLRU(10)
	c.put("a", []byte("aaaa"))
	c.put("b", []byte("bbbb"))
	c.get("a")
	c.put("c", []byte("cccc")) // evicts b, the least recently used
	if _, ok := c.get("b"); ok {
		t.Error("b should be evicted")
	}
	for _, k := range []string{"a", "c"} {
		if _, ok := c.get(k); !ok {
			t.Errorf("%s missing", k)
		}
	}
	c.put("huge", bytes.Repeat([]byte("x"), 11))
	if _, ok := c.get("huge"); ok {
		t.Error("entries over the budget are not cached")
	}
}

// TestJailedPipeline runs the real gzmesh pipeline under bubblewrap. It
// needs a Python with the meshworker requirements:
//
//	MESHWORKER_IT_PYTHON=/path/to/venv/bin/python MESHWORKER_JAIL_RO=/venv:/uv-python go test ./cmd/meshworker
func TestJailedPipeline(t *testing.T) {
	py := os.Getenv("MESHWORKER_IT_PYTHON")
	if py == "" {
		t.Skip("set MESHWORKER_IT_PYTHON to run the pipeline under bwrap")
	}
	gz, _ := filepath.Abs("../../meshworker")
	_, srv, _ := newTestWorker(t, func(c *config) {
		c.Jail, c.Python, c.GzmeshDir = "bwrap", py, gz
		c.ExtraRO = append(filepath.SplitList(os.Getenv("MESHWORKER_JAIL_RO")), gz)
		c.MaxOutput = 30 << 20
		c.JobTimeout = 120 * time.Second
	})
	code := "from build123d import *\nimport gzmesh as gz\n\ndef build():\n    return gz.part(Pos(0, 0, 0.1) * Box(0.4, 0.3, 0.2), 'crate', gz.cardboard())\n"
	resp := post(t, srv.URL+"/v1/jobs", jobJSON(code))
	if resp.Header.Get("Content-Type") != "application/x-tar" {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	names, files := untar(t, resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(files["report.json"], `"ok": true`) {
		t.Fatalf("%d %v\n%s", resp.StatusCode, names, files["report.json"])
	}

	// The allow-list is not the security boundary (numpy, for one, can read
	// files); the jail is. Run a probe with the jail's exact arguments.
	probeJail(t, py, gz)

}

// probeJail runs Python under the same bwrap arguments as a job and checks
// that files outside the job directory and the network are out of reach.
// The same probe without the jail must see the secret, or it proves nothing.
func probeJail(t *testing.T, py, gz string) {
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("hunter2"), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := `
import socket, sys
for p in sys.argv[1:]:
    try:
        open(p).read(1)
        print("LEAK file", p)
    except OSError as e:
        print("blocked", p, type(e).__name__)
try:
    socket.create_connection(("1.1.1.1", 53), timeout=2)
    print("LEAK network")
except OSError as e:
    print("blocked network", type(e).__name__)
`
	paths := []string{secret, os.Getenv("HOME") + "/.bashrc", "/root/.bashrc"}
	run := func(jail string) string {
		w, _, _ := newTestWorker(t, func(c *config) {
			c.Jail, c.Python, c.GzmeshDir = jail, py, gz
			c.ExtraRO = filepath.SplitList(os.Getenv("MESHWORKER_JAIL_RO"))
		})
		dir := t.TempDir()
		cmd := w.jailed(dir, 0)
		args := cmd.Args
		for i, a := range args {
			if a == py { // replace "python -m gzmesh.run ..." with the probe
				args = append(append([]string{}, args[:i+1]...), append([]string{"-c", probe}, paths...)...)
				break
			}
		}
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s probe: %v\n%s", jail, err, out)
		}
		return string(out)
	}
	if out := run("none"); !strings.Contains(out, "LEAK file "+secret) {
		t.Fatalf("without the jail the probe should read the secret:\n%s", out)
	}
	out := run("bwrap")
	if strings.Contains(out, "LEAK") || strings.Count(out, "blocked") != len(paths)+1 {
		t.Fatalf("jail leaks:\n%s", out)
	}
	t.Logf("jail probe:\n%s", out)
}

func TestSlotsPinnedToCPUPairs(t *testing.T) {
	w := &worker{cfg: config{CPUs: []int{0, 1, 2, 3, 8, 9}, Jail: "bwrap", Python: "python3"}}
	for slot, want := range []string{"0,1", "2,3", "8,9", "0,1"} {
		if got := w.slotCPUs(slot); got != want {
			t.Errorf("slot %d: %q want %q", slot, got, want)
		}
	}
	args := strings.Join(w.jailed(t.TempDir(), 1).Args, " ")
	if !strings.Contains(args, "taskset -c 2,3 prlimit") {
		t.Errorf("jail command: %s", args)
	}
	if (&worker{cfg: config{CPUs: []int{0}}}).slotCPUs(0) != "" {
		t.Error("one CPU: no pinning")
	}
}
