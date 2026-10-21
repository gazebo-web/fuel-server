package aitools

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"time"

	"github.com/gazebo-web/fuel-server/bundles/projects"
)

// Inspecting uploaded project files on the mesh worker (its /v1/inspect):
// PDF pages, drawings, mesh previews and texture swatches, made in its
// sandbox, for asset_view to show. projects.AddAssets calls it.

// inspectMaxTar caps an inspect result: 20 page images and the rest.
const inspectMaxTar = 64 << 20

// viewNameRE is a view the worker may return.
var viewNameRE = regexp.MustCompile(`^(ai\.jpg|thumb\.jpg|preview\.png|page-[0-9]{1,3}\.jpg)$`)

// Inspections share the worker with model jobs, so each user gets a
// budget of them (separate from model_generate's).
const (
	inspectPerMinute = 30
	inspectPerDay    = 500
)

// inspector is projects.InspectAsset: the worker's inspect job, rate
// limited per user. Failures that aren't the file's are retryable.
func (m *meshClient) inspector() projects.AssetInspector {
	limit := newRateLimiter(inspectPerMinute, inspectPerDay)
	return func(ctx context.Context, user, kind, file, main string, data []byte) (projects.AssetInfo, map[string][]byte, error) {
		if wait, ok := limit.allow(user, time.Now()); !ok {
			return nil, nil, projects.RetryableError{Err: fmt.Errorf("too many files to preview (%d a minute, %d a day); "+
				"try again in %s", inspectPerMinute, inspectPerDay, wait.Round(time.Second))}
		}
		if kind == projects.AssetModel {
			return m.measureModel(ctx, data)
		}
		return m.inspect(ctx, kind, file, main, data)
	}
}

// measureModel measures an uploaded Gazebo model (a zip of its folder) as
// the worker measures Fuel models: its size as Gazebo shows it.
func (m *meshClient) measureModel(ctx context.Context, zip []byte) (projects.AssetInfo, map[string][]byte, error) {
	body, err := m.measure(ctx, zip)
	if err != nil {
		return nil, nil, projects.RetryableError{Err: fmt.Errorf("the model couldn't be measured: %v", err)}
	}
	var r measureReport
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, nil, projects.RetryableError{Err: fmt.Errorf("mesh worker: bad report: %v", err)}
	}
	if !r.OK {
		return nil, nil, fmt.Errorf("the model couldn't be measured: %s", r.Error)
	}
	info := projects.AssetInfo{"size": r.Size[:], "bbox": map[string]any{"min": r.BBox.Min[:], "max": r.BBox.Max[:]}}
	if len(r.Warnings) > 0 {
		info["measure_warnings"] = r.Warnings
	}
	return info, nil, nil
}

// inspect posts an upload to the worker and returns its info and views.
// A file the worker can't read is an error saying why; the worker being
// busy, down or slow is a projects.RetryableError.
func (m *meshClient) inspect(ctx context.Context, kind, file, main string, data []byte) (projects.AssetInfo,
	map[string][]byte, error) {
	retry := func(err error) error { return projects.RetryableError{Err: err} }

	q := url.Values{"kind": {kind}, "file": {file}, "wait": {"90"}}
	if main != "" {
		q.Set("main", main)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.base+"/v1/inspect?"+q.Encode(), bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	for {
		if m.token != "" {
			req.Header.Set("Authorization", "Bearer "+m.token)
		}
		resp, err := m.http.Do(req)
		if err != nil {
			return nil, nil, retry(errors.New("the preview couldn't be made: the mesh worker is unavailable"))
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, inspectMaxTar+1))
		resp.Body.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("mesh worker: %w", err)
		}
		if resp.Header.Get("Content-Type") == "application/x-tar" {
			info, views, err := readInspection(body)
			if err != nil && resp.StatusCode != http.StatusOK {
				err = retry(err) // the worker's own failure (exit 1), not the file's
			}
			return info, views, err
		}
		var msg struct {
			Error string `json:"error"`
			JobID string `json:"job_id"`
		}
		_ = json.Unmarshal(body, &msg)
		switch resp.StatusCode {
		case http.StatusAccepted:
			if req, err = http.NewRequestWithContext(ctx, http.MethodGet, m.base+"/v1/jobs/"+url.PathEscape(msg.JobID), nil); err != nil {
				return nil, nil, err
			}
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return nil, nil, retry(errors.New("the preview took too long to make"))
			}
		case http.StatusTooManyRequests:
			return nil, nil, retry(errors.New("the preview couldn't be made: the mesh worker is busy"))
		case http.StatusRequestEntityTooLarge:
			return nil, nil, errors.New("the file is too large to preview")
		default:
			if msg.Error == "" {
				msg.Error = fmt.Sprintf("status %d", resp.StatusCode)
			}
			err := fmt.Errorf("the preview couldn't be made: %s", msg.Error)
			if resp.StatusCode >= 500 {
				return nil, nil, retry(err)
			}
			return nil, nil, err
		}
	}
}

func readInspection(tarball []byte) (projects.AssetInfo, map[string][]byte, error) {
	var report struct {
		OK    bool               `json:"ok"`
		Error string             `json:"error"`
		Info  projects.AssetInfo `json:"info"`
	}
	views := map[string][]byte{}
	found := false
	tr := tar.NewReader(bytes.NewReader(tarball))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("mesh worker: bad result: %w", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, nil, fmt.Errorf("mesh worker: bad result: %w", err)
		}
		switch name := path.Clean(h.Name); {
		case name == "report.json" && len(data) > 1<<20:
			return nil, nil, errors.New("mesh worker: report too large")
		case name == "report.json":
			if err := json.Unmarshal(data, &report); err != nil {
				return nil, nil, fmt.Errorf("mesh worker: bad report: %w", err)
			}
			found = true
		case viewNameRE.MatchString(name):
			views[name] = data
		default:
			return nil, nil, fmt.Errorf("mesh worker: unexpected file %s", h.Name)
		}
	}
	switch {
	case !found:
		return nil, nil, errors.New("mesh worker: no report")
	case !report.OK:
		return nil, nil, errors.New(report.Error)
	}
	return report.Info, views, nil
}
