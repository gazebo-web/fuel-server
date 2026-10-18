package aitools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"

	"github.com/gazebo-web/fuel-server/globals"
)

// Sizes of Fuel models (plan §5.6). Agents place library models by their
// size: without it they guess, and a pallet ends up inside the floor or a
// shelf floats. The mesh worker measures a model's bounding box from its
// zip as Gazebo would show it (gzmesh/measure.py), and the result is kept
// in ai_model_sizes until the model changes.
//
// Models are measured when search returns them (a few at a time, briefly
// waiting), when fuel_get_model_details asks for one, and, for this
// server's own public models, by a background sweep, so in production
// most sizes are already known when an agent searches.

const (
	// How long search waits for new measurements, and how many it starts.
	sizeSearchWait = 15 * time.Second
	sizeSearchMax  = 8
	// How long fuel_get_model_details waits.
	sizeDetailsWait = 90 * time.Second
	// A measurement's own deadline: download plus worker.
	sizeMeasureTimeout = 3 * time.Minute
	// Parallel measurements per server.
	sizeParallel = 4
	// A failed download or worker call is retried after this long.
	sizeRetryAfter = 10 * time.Minute
	// Recent results kept in memory, in front of the database.
	sizeMemoMax = 10000
	// The largest model zip downloaded to measure (the worker's limit).
	sizeMaxZip = 256 << 20
	// The background sweep of this server's own models.
	sizeSweepIdle  = 10 * time.Minute
	sizeSweepRetry = time.Minute
	sizeSweepPause = time.Second
	sizeSweepBatch = 50
)

// ModelSize is the measured bounding box of one Fuel model, keyed by the
// catalog host, owner and name. Stamp is the model's modify date when it
// was measured; a model modified since is measured again. Report is the
// worker's report (gzmesh/measure.py), including failed ones, so a model
// that can't be measured isn't retried until it changes.
type ModelSize struct {
	ID        uint `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Host      string `gorm:"type:varchar(190);unique_index:idx_ai_model_size"`
	Owner     string `gorm:"type:varchar(190);unique_index:idx_ai_model_size"`
	Name      string `gorm:"type:varchar(190);unique_index:idx_ai_model_size"`
	Stamp     string `gorm:"type:varchar(64)"`
	Report    string `gorm:"type:text"`
}

// TableName keeps the AI Builder's table apart from Fuel's own.
func (ModelSize) TableName() string { return "ai_model_sizes" }

// measureReport is the part of the worker's report the tools use.
type measureReport struct {
	OK   bool `json:"ok"`
	BBox struct {
		Min [3]float64 `json:"min"`
		Max [3]float64 `json:"max"`
	} `json:"bbox"`
	Size     [3]float64 `json:"size"`
	Visuals  int        `json:"visuals"`
	Measured int        `json:"measured"`
	From     string     `json:"from"`
	Warnings []string   `json:"warnings"`
	Error    string     `json:"error"`
}

// BBox is a model's size as the tools return it: the box around its
// visuals in meters, in the model's own frame.
type BBox struct {
	Size [3]float64 `json:"size"`
	Min  [3]float64 `json:"min"`
	Max  [3]float64 `json:"max"`
	// Note says when the box may be incomplete.
	Note string `json:"note,omitempty"`
}

func (r measureReport) bbox() *BBox {
	if !r.OK {
		return nil
	}
	b := &BBox{Size: r.Size, Min: r.BBox.Min, Max: r.BBox.Max}
	switch {
	case r.From == "collision":
		b.Note = "measured from collisions; the model has no measurable visuals"
	case r.Measured < r.Visuals:
		b.Note = fmt.Sprintf("%d of %d visuals measured", r.Measured, r.Visuals)
	}
	return b
}

type sizeKey struct{ host, owner, name string }

func (k sizeKey) String() string { return k.host + "/" + k.owner + "/" + k.name }

// sizer measures models through the mesh worker and caches the results.
type sizer struct {
	worker *meshClient
	// db is where sizes are kept; nil means globals.Server.Db.
	db  *gorm.DB
	sem chan struct{}

	mu       sync.Mutex
	inflight map[sizeKey]*sizeCall
	failed   map[sizeKey]time.Time
	memo     map[sizeKey]memo
}

type memo struct {
	stamp  string
	report *measureReport
}

type sizeCall struct {
	done   chan struct{}
	report *measureReport
	err    error
}

func newSizer(worker *meshClient) *sizer {
	return &sizer{worker: worker, sem: make(chan struct{}, sizeParallel),
		inflight: map[sizeKey]*sizeCall{}, failed: map[sizeKey]time.Time{}, memo: map[sizeKey]memo{}}
}

func (s *sizer) database() *gorm.DB {
	if s.db != nil {
		return s.db
	}
	if globals.Server != nil {
		return globals.Server.Db
	}
	return nil
}

// stored returns the kept sizes for keys whose stamp still matches: a nil
// *BBox means the model was measured and has no size.
func (s *sizer) stored(keys map[sizeKey]string) map[sizeKey]*measureReport {
	out := map[sizeKey]*measureReport{}
	current := func(want, have string) bool { return want == "" || normStamp(want) == have }
	var hosts, owners, names []string
	s.mu.Lock()
	for k, stamp := range keys {
		if m, ok := s.memo[k]; ok && current(stamp, m.stamp) {
			out[k] = m.report
			continue
		}
		hosts, owners, names = append(hosts, k.host), append(owners, k.owner), append(names, k.name)
	}
	s.mu.Unlock()
	db := s.database()
	if db == nil || len(hosts) == 0 {
		return out
	}
	var rows []ModelSize
	if err := db.Where("host IN (?) AND owner IN (?) AND name IN (?)", hosts, owners, names).
		Find(&rows).Error; err != nil {
		log.Printf("aitools: read model sizes: %v", err)
		return out
	}
	for _, row := range rows {
		k := sizeKey{row.Host, row.Owner, row.Name}
		stamp, ok := keys[k]
		if !ok || !current(stamp, row.Stamp) {
			continue
		}
		var r measureReport
		if json.Unmarshal([]byte(row.Report), &r) == nil {
			out[k] = &r
			s.remember(k, row.Stamp, &r)
		}
	}
	return out
}

func (s *sizer) remember(k sizeKey, stamp string, r *measureReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.memo) >= sizeMemoMax {
		s.memo = map[sizeKey]memo{}
	}
	s.memo[k] = memo{stamp, r}
}

// measure returns the model's report, measuring it (or joining a running
// measurement) and waiting at most wait. A model that can't be measured
// returns a report saying why; errors are about getting the answer.
func (s *sizer) measure(cat Catalog, k sizeKey, stamp string, wait time.Duration) (*measureReport, error) {
	s.mu.Lock()
	if t, ok := s.failed[k]; ok && time.Since(t) < sizeRetryAfter {
		s.mu.Unlock()
		return nil, errors.New("measuring failed recently; try again later")
	}
	c, running := s.inflight[k]
	if !running {
		c = &sizeCall{done: make(chan struct{})}
		s.inflight[k] = c
		go s.run(cat, k, stamp, c)
	}
	s.mu.Unlock()
	select {
	case <-c.done:
		return c.report, c.err
	case <-time.After(wait):
		return nil, errStillMeasuring
	}
}

var errStillMeasuring = errors.New("still measuring")

// run measures one model on its own deadline, so a measurement outlives
// the request that started it and the next search finds it stored.
func (s *sizer) run(cat Catalog, k sizeKey, stamp string, c *sizeCall) {
	s.sem <- struct{}{}
	defer func() { <-s.sem }()
	ctx, cancel := context.WithTimeout(context.Background(), sizeMeasureTimeout)
	defer cancel()
	report, err := s.measureNow(ctx, cat, k)
	if err == nil {
		err = s.store(k, stamp, report)
	}
	s.mu.Lock()
	delete(s.inflight, k)
	if err != nil {
		s.failed[k] = time.Now()
		log.Printf("aitools: measure %s: %v", k, err)
	}
	s.mu.Unlock()
	c.report, c.err = report, err
	close(c.done)
}

func (s *sizer) measureNow(ctx context.Context, cat Catalog, k sizeKey) (*measureReport, error) {
	zip, err := cat.Zip(ctx, s.database(), k.owner, k.name)
	if errors.Is(err, errTooLarge) {
		return &measureReport{Error: "the model is too large to measure"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	body, err := s.worker.measure(ctx, zip)
	if err != nil {
		return nil, err
	}
	var r measureReport
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("worker report: %w", err)
	}
	return &r, nil
}

func (s *sizer) store(k sizeKey, stamp string, r *measureReport) error {
	s.remember(k, normStamp(stamp), r)
	db := s.database()
	if db == nil {
		return nil
	}
	b, _ := json.Marshal(r)
	row := ModelSize{Host: k.host, Owner: k.owner, Name: k.name}
	return db.Where(row).Assign(ModelSize{Stamp: normStamp(stamp), Report: string(b)}).FirstOrCreate(&row).Error
}

// fill sets the size of each public model in items: stored ones at once,
// and up to sizeSearchMax others measured while waiting at most wait.
func (s *sizer) fill(items []catalogItem, wait time.Duration) {
	keys := map[sizeKey]string{}
	for _, it := range items {
		if it.Public && it.cat != nil {
			keys[sizeKey{it.cat.Host(), it.Owner, it.Name}] = it.stamp
		}
	}
	have := s.stored(keys)
	type pending struct {
		i    int
		done chan struct{}
	}
	var started []pending
	deadline := time.Now().Add(wait)
	for i := range items {
		it := &items[i]
		if !it.Public || it.cat == nil {
			continue
		}
		k := sizeKey{it.cat.Host(), it.Owner, it.Name}
		if r, ok := have[k]; ok {
			it.setSize(r.bbox(), r.Error)
			continue
		}
		if len(started) >= sizeSearchMax {
			it.SizeNote = "not measured yet; fuel_get_model_details measures it"
			continue
		}
		p := pending{i: i, done: make(chan struct{})}
		started = append(started, p)
		go func(cat Catalog, stamp string) {
			defer close(p.done)
			r, err := s.measure(cat, k, stamp, time.Until(deadline))
			items[p.i].setMeasured(r, err)
		}(it.cat, it.stamp)
	}
	for _, p := range started {
		<-p.done
	}
}

func (it *catalogItem) setSize(b *BBox, failure string) {
	if b != nil {
		it.BBox = b
		return
	}
	if failure == "" {
		failure = "the model couldn't be measured"
	}
	it.SizeNote = "size unknown: " + failure
}

func (it *catalogItem) setMeasured(r *measureReport, err error) {
	switch {
	case errors.Is(err, errStillMeasuring):
		it.SizeNote = "still measuring; search again or call fuel_get_model_details"
	case err != nil:
		it.SizeNote = "size unknown: " + err.Error()
	default:
		it.setSize(r.bbox(), r.Error)
	}
}

// sweep measures this server's public models that have no current size,
// a batch at a time, then idles; it covers new uploads and edits too.
func (s *sizer) sweep(cat Catalog) {
	// The registry is built before the server migrates its tables.
	time.Sleep(sizeSweepRetry)
	for {
		todo, measured, err := s.sweepBatch(cat, sizeSweepBatch)
		switch {
		case err != nil:
			log.Printf("aitools: size sweep: %v", err)
			time.Sleep(sizeSweepRetry)
		// Models that keep failing stay unmeasured; idle rather than
		// retrying them back to back.
		case todo < sizeSweepBatch || measured == 0:
			time.Sleep(sizeSweepIdle)
		}
	}
}

// sweepBatch measures up to n public models of cat's server that have no
// current size, one at a time.
func (s *sizer) sweepBatch(cat Catalog, n int) (todo, measured int, err error) {
	db := s.database()
	if db == nil {
		return 0, 0, errors.New("no database yet")
	}
	refs, err := unmeasuredModels(db, cat.Host(), n)
	for _, m := range refs {
		if _, err := s.measure(cat, sizeKey{cat.Host(), m.owner, m.name}, m.stamp, sizeMeasureTimeout); err == nil {
			measured++
		}
		time.Sleep(sizeSweepPause)
	}
	return len(refs), measured, err
}

// MeasureLocalModels measures up to n of this server's public models that
// have no current size, as the background sweep does; for backfills and
// tests. It returns how many needed measuring and how many were measured.
func MeasureLocalModels(cfg Config, db *gorm.DB, n int) (todo, measured int, err error) {
	if cfg.MeshWorkerURL == "" {
		return 0, 0, errors.New("no mesh worker configured")
	}
	s := newSizer(newMeshClient(cfg))
	s.db = db
	return s.sweepBatch(catalogsFor(cfg)[0], n)
}

func newMeshClient(cfg Config) *meshClient {
	return &meshClient{base: strings.TrimSuffix(cfg.MeshWorkerURL, "/"), token: cfg.MeshWorkerToken,
		http: &http.Client{Timeout: 2 * time.Minute}}
}

type modelRef struct{ owner, name, stamp string }

// unmeasuredModels lists public models with no size, or one older than the
// model, skipping models that failed recently (measure skips those too).
func unmeasuredModels(db *gorm.DB, host string, n int) ([]modelRef, error) {
	rows, err := db.Raw(`SELECT m.owner, m.name, m.modify_date FROM models m
		LEFT JOIN ai_model_sizes s ON s.host = ? AND s.owner = m.owner AND s.name = m.name
		WHERE m.deleted_at IS NULL AND (m.private IS NULL OR m.private = false)
		  AND (s.id IS NULL OR s.updated_at < m.modify_date)
		ORDER BY m.id LIMIT ?`, host, n).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []modelRef
	for rows.Next() {
		var owner, name string
		var modified *time.Time
		if err := rows.Scan(&owner, &name, &modified); err != nil {
			return nil, err
		}
		ref := modelRef{owner: owner, name: name}
		if modified != nil {
			ref.stamp = modified.UTC().Format(time.RFC3339)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// normStamp puts a modify date in one form, whichever way it was written
// (the REST API's RFC 3339, the database's time).
func normStamp(s string) string {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return strings.TrimSpace(s)
}

// worldBox is the axis-aligned box a model with box b covers when placed
// at pose ("x y z roll pitch yaw").
func worldBox(b *BBox, pose string) (min, max [3]float64, ok bool) {
	var p [6]float64
	if f := strings.Fields(pose); len(f) == 6 {
		for i, s := range f {
			if _, err := fmt.Sscan(s, &p[i]); err != nil {
				return min, max, false
			}
		}
	} else if strings.TrimSpace(pose) != "" {
		return min, max, false
	}
	r := rpy(p[3], p[4], p[5])
	for i := 0; i < 3; i++ {
		min[i], max[i] = math.Inf(1), math.Inf(-1)
	}
	for c := 0; c < 8; c++ {
		corner := [3]float64{pick(c&1, b.Min[0], b.Max[0]), pick(c&2, b.Min[1], b.Max[1]), pick(c&4, b.Min[2], b.Max[2])}
		for i := 0; i < 3; i++ {
			v := p[i] + r[i][0]*corner[0] + r[i][1]*corner[1] + r[i][2]*corner[2]
			min[i], max[i] = math.Min(min[i], v), math.Max(max[i], v)
		}
	}
	for i := 0; i < 3; i++ {
		min[i], max[i] = round4(min[i]), round4(max[i])
	}
	return min, max, true
}

func pick(bit int, a, b float64) float64 {
	if bit != 0 {
		return b
	}
	return a
}

func round4(v float64) float64 { return math.Round(v*1e4)/1e4 + 0 }

func rpy(r, p, y float64) [3][3]float64 {
	cr, sr, cp, sp, cy, sy := math.Cos(r), math.Sin(r), math.Cos(p), math.Sin(p), math.Cos(y), math.Sin(y)
	return [3][3]float64{
		{cy * cp, cy*sp*sr - sy*cr, cy*sp*cr + sy*sr},
		{sy * cp, sy*sp*sr + cy*cr, sy*sp*cr - cy*sr},
		{-sp, cp * sr, cp * cr},
	}
}

// -- the worker ---------------------------------------------------------------

// meshClient calls the mesh worker.
type meshClient struct {
	base, token string
	http        *http.Client
}

// measure posts a model zip to the worker and returns its report,
// following a 202 until the job finishes or ctx ends.
func (m *meshClient) measure(ctx context.Context, zip []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.base+"/v1/measure?wait=90", bytes.NewReader(zip))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/zip")
	for {
		if m.token != "" {
			req.Header.Set("Authorization", "Bearer "+m.token)
		}
		resp, err := m.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("mesh worker: %w", err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("mesh worker: %w", err)
		}
		switch resp.StatusCode {
		case http.StatusOK:
			return body, nil
		case http.StatusAccepted:
			var j struct {
				JobID string `json:"job_id"`
			}
			if json.Unmarshal(body, &j) != nil || j.JobID == "" {
				return nil, fmt.Errorf("mesh worker: 202 without a job id")
			}
			if req, err = http.NewRequestWithContext(ctx, http.MethodGet, m.base+"/v1/jobs/"+j.JobID, nil); err != nil {
				return nil, err
			}
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case http.StatusRequestEntityTooLarge:
			return json.Marshal(measureReport{Error: "the model is too large to measure"})
		default:
			return nil, fmt.Errorf("mesh worker: status %d: %s", resp.StatusCode, truncate(string(body), 200))
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
