// Command meshworker is the AI Builder's mesh worker front end (plan §5.1
// in docs/ai-builder-models-and-meshes-plan.md). It takes a job, which is
// agent-written build123d code plus parameters, runs the gzmesh pipeline
// on it in a per-job sandbox, and returns the model folder as a tarball.
// It is stateless and holds no credentials: fuel-server commits the result.
//
//	POST /v1/jobs?wait=90   job JSON -> 200 tar | 202 {"job_id"} | 429 | 400
//	GET  /v1/jobs/{id}      202 {"status":"running"} | 200 tar | 404
//	GET  /healthz
//
// Every tar holds report.json (the pipeline's report; "ok" says whether the
// model was built) and, on success, preview.png and models/<name>/....
//
// Configuration (environment):
//
//	MESHWORKER_ADDR         listen address (":8020")
//	MESHWORKER_TOKEN        shared secret callers send as "Authorization: Bearer"; unset disables auth (dev)
//	MESHWORKER_PYTHON       Python with the gzmesh dependencies ("python3")
//	MESHWORKER_GZMESH       directory containing the gzmesh package ("/opt/meshworker")
//	MESHWORKER_JAIL         "bwrap" (default) or "none" (tests and local hacking only)
//	MESHWORKER_WORKDIR      job directories (os.TempDir()/meshworker)
//	MESHWORKER_JAIL_RO      extra read-only paths for the jail, ":"-separated (a dev venv and its interpreter)
//	MESHWORKER_CONCURRENCY  parallel jobs (NumCPU/2, at least 1)
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

func main() {
	cfg := defaultConfig()
	cfg.Addr = env("MESHWORKER_ADDR", ":8020")
	cfg.Token = os.Getenv("MESHWORKER_TOKEN")
	cfg.Python = env("MESHWORKER_PYTHON", cfg.Python)
	cfg.GzmeshDir = env("MESHWORKER_GZMESH", cfg.GzmeshDir)
	cfg.Jail = env("MESHWORKER_JAIL", cfg.Jail)
	cfg.WorkDir = env("MESHWORKER_WORKDIR", cfg.WorkDir)
	if ro := os.Getenv("MESHWORKER_JAIL_RO"); ro != "" {
		cfg.ExtraRO = filepath.SplitList(ro)
	}
	if n, err := strconv.Atoi(os.Getenv("MESHWORKER_CONCURRENCY")); err == nil && n > 0 {
		cfg.Concurrency = n
		cfg.QueueLen = 2 * n
	}
	if cfg.Token == "" {
		log.Println("meshworker: MESHWORKER_TOKEN unset; accepting unauthenticated jobs (dev only)")
	}
	if cfg.Jail == "none" {
		log.Println("meshworker: MESHWORKER_JAIL=none; model code runs without a sandbox (dev only)")
	}
	w, err := newWorker(cfg)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("meshworker: listening on %s, %d parallel jobs, jail %s", cfg.Addr, cfg.Concurrency, cfg.Jail)
	srv := &http.Server{Addr: cfg.Addr, Handler: w.routes(), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func defaultConfig() config {
	n := runtime.NumCPU() / 2
	if n < 1 {
		n = 1
	}
	return config{
		Python:      "python3",
		GzmeshDir:   "/opt/meshworker",
		Jail:        "bwrap",
		WorkDir:     filepath.Join(os.TempDir(), "meshworker"),
		Concurrency: n,
		QueueLen:    2 * n,
		JobTimeout:  120 * time.Second,
		MaxWait:     110 * time.Second,
		DefaultWait: 90 * time.Second,
		KeepResults: 10 * time.Minute,
		CacheBytes:  1 << 30,
		MaxOutput:   30 << 20,
		MaxJobBytes: 256 << 10,
		// Address space, not memory: thread pools reserve far more than
		// they touch (a multi-link job peaks near 1 GB resident but 9 GB
		// virtual). The container's memory limit caps real use.
		MemoryBytes: 16 << 30,
		CPUs:        allCPUs(),
		CPUSeconds:  150,
	}
}

// allCPUs lists the processors this process may run on.
func allCPUs() []int {
	var set unix.CPUSet
	if err := unix.SchedGetaffinity(0, &set); err != nil {
		out := make([]int, runtime.NumCPU())
		for i := range out {
			out[i] = i
		}
		return out
	}
	var out []int
	for i := 0; i < 1024 && len(out) < set.Count(); i++ {
		if set.IsSet(i) {
			out = append(out, i)
		}
	}
	return out
}
