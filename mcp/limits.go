package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Rate limits on the public endpoint (design §8.4, Q-6), per caller (user
// and credential): tool calls overall, searches, and calls that write.
// Bursts up to twice the per-minute rate are allowed. Over a limit, the
// request gets 429 with Retry-After.
type Limits struct {
	CallsPerMinute, SearchesPerMinute, WritesPerMinute int
}

// DefaultLimits are the design's starting values, to tune from logs.
var DefaultLimits = Limits{CallsPerMinute: 120, SearchesPerMinute: 30, WritesPerMinute: 60}

// Limiter enforces Limits for many callers.
type Limiter struct {
	limits Limits
	reg    *Registry
	now    func() time.Time

	mu      sync.Mutex
	callers map[string]*callerLimits
	sweep   time.Time
}

type callerLimits struct {
	calls, searches, writes *rate.Limiter
	used                    time.Time
}

const limiterIdle = 10 * time.Minute

// MaxBodyBytes is the largest request the endpoint reads: room for a
// project file at its 10 MB limit, JSON-escaped.
const MaxBodyBytes = 16 << 20

// NewLimiter returns a limiter; reg tells which tools write.
func NewLimiter(l Limits, reg *Registry) *Limiter {
	return &Limiter{limits: l, reg: reg, now: time.Now, callers: map[string]*callerLimits{}}
}

func perMinute(n int) *rate.Limiter {
	return rate.NewLimiter(rate.Limit(float64(n)/60), 2*n)
}

// Allow counts one call of tool by caller and returns 0, or how long to
// wait when a limit is used up (nothing is counted then).
func (l *Limiter) Allow(caller, tool string) time.Duration {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.sweep) > limiterIdle {
		for k, c := range l.callers {
			if now.Sub(c.used) > limiterIdle {
				delete(l.callers, k)
			}
		}
		l.sweep = now
	}
	c := l.callers[caller]
	if c == nil {
		c = &callerLimits{calls: perMinute(l.limits.CallsPerMinute), searches: perMinute(l.limits.SearchesPerMinute),
			writes: perMinute(l.limits.WritesPerMinute)}
		l.callers[caller] = c
	}
	c.used = now
	buckets := []*rate.Limiter{c.calls}
	if strings.HasPrefix(tool, "fuel_search_") {
		buckets = append(buckets, c.searches)
	}
	if t, ok := l.reg.Get(tool); ok && !t.Annotations.ReadOnly {
		buckets = append(buckets, c.writes)
	}
	var wait time.Duration
	var taken []*rate.Reservation
	for _, b := range buckets {
		r := b.ReserveN(now, 1)
		taken = append(taken, r)
		if d := r.DelayFrom(now); d > wait {
			wait = d
		}
	}
	if wait > 0 {
		for _, r := range taken {
			r.CancelAt(now)
		}
	}
	return wait
}

// Middleware limits the tools/call requests in JSON-RPC bodies; key names
// the caller of a request.
func (l *Limiter) Middleware(key func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
			r.Body.Close()
			if err != nil {
				http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
				return
			}
			if len(body) > MaxBodyBytes {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			k := key(r)
			for _, tool := range calledTools(body) {
				if wait := l.Allow(k, tool); wait > 0 {
					secs := int(math.Ceil(wait.Seconds()))
					w.Header().Set("Retry-After", strconv.Itoa(secs))
					http.Error(w, "rate limit: too many calls; retry in "+strconv.Itoa(secs)+"s",
						http.StatusTooManyRequests)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// calledTools are the tool names of the tools/call messages in a JSON-RPC
// body (one message or a batch). Anything unparsable is left for the MCP
// server to reject.
func calledTools(body []byte) []string {
	type msg struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	var msgs []msg
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		_ = json.Unmarshal(trimmed, &msgs)
	} else {
		var m msg
		if json.Unmarshal(trimmed, &m) == nil {
			msgs = []msg{m}
		}
	}
	var out []string
	for _, m := range msgs {
		if m.Method == "tools/call" && m.Params.Name != "" {
			out = append(out, m.Params.Name)
		}
	}
	return out
}
