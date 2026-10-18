package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The Gazebo check: download a case's project (world.sdf and its project
// models), run it headless with gzcheck/gzcheck.sh, and require data on
// the kinds of data the case names (image, scan, imu, odometry, ...),
// whatever topic carries them. Validation can say a sensor is declared;
// only running the world says it publishes.

type treeNode struct {
	Path     string      `json:"path"`
	Children []*treeNode `json:"children"`
}

func (r *runner) gazeboCheck(ctx context.Context, project string, want []string, res *Result) {
	dir := filepath.Join(r.gazeboDir, project)
	if err := r.download(ctx, project, dir); err != nil {
		res.Failures = append(res.Failures, "gazebo: download: "+err.Error())
		return
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, r.gzcheck, dir, "10")
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		res.Failures = append(res.Failures, fmt.Sprintf("gazebo: %v: %s", err, truncate(out.String(), 300)))
		return
	}
	var silent []string
	kinds := map[string]bool{}
	sc := bufio.NewScanner(&out)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 3 {
			continue
		}
		switch f[0] {
		case "ok":
			kinds[f[1]] = true
			res.Topics = append(res.Topics, f[2])
		case "silent":
			silent = append(silent, f[2])
		}
	}
	for _, kind := range want {
		if !kinds[kind] {
			res.Failures = append(res.Failures, fmt.Sprintf("gazebo: no %s data (silent: %s)", kind,
				strings.Join(silent, " ")))
		}
	}
}

// download writes the project's world.sdf and models/ under dir.
func (r *runner) download(ctx context.Context, project, dir string) error {
	p := url.PathEscape(project)
	var b []byte
	if err := r.call(ctx, "GET", r.fuel+"/1.0/projects/"+p+"/file-url", nil, &b); err != nil {
		return err
	}
	var signed struct {
		BasePath string `json:"base_path"`
	}
	if err := json.Unmarshal(b, &signed); err != nil {
		return err
	}
	base := r.fuel + signed.BasePath
	save := func(u, rel string) error {
		var data []byte
		if err := r.call(ctx, "GET", u, nil, &data); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, data, 0o644)
	}
	if err := save(base+"/files/world.sdf", "world.sdf"); err != nil {
		return err
	}
	if err := r.call(ctx, "GET", r.fuel+"/1.0/projects/"+p+"/models", nil, &b); err != nil {
		return err
	}
	var models struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &models); err != nil {
		return err
	}
	for _, m := range models.Models {
		mu := base + "/models/" + url.PathEscape(m.Name) + "/tip/files"
		if err := r.call(ctx, "GET", mu, nil, &b); err != nil {
			return err
		}
		var tree struct {
			FileTree []*treeNode `json:"file_tree"`
		}
		if err := json.Unmarshal(b, &tree); err != nil {
			return err
		}
		var walk func([]*treeNode) error
		walk = func(nodes []*treeNode) error {
			for _, n := range nodes {
				if len(n.Children) > 0 {
					if err := walk(n.Children); err != nil {
						return err
					}
					continue
				}
				rel := strings.TrimPrefix(n.Path, "/")
				if err := save(mu+"/"+rel, "models/"+m.Name+"/"+rel); err != nil {
					return err
				}
			}
			return nil
		}
		if err := walk(tree.FileTree); err != nil {
			return err
		}
	}
	return nil
}
