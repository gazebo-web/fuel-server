// Package sdf provides SDF schema parsing, lookup, and lightweight
// XML-structural validation. The sdformat XML schemas are embedded at
// build time (go:embed) so a running fuel-server doesn't need a separate
// schema directory on disk. This is the "schema-as-context" source the
// AI Builder uses to seed its system prompts (design §13.10).
package sdf

import (
	"embed"
	"encoding/xml"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
)

//go:embed schemas
var schemaFS embed.FS

// Element is one element definition in an SDF schema.
type Element struct {
	XMLName     xml.Name    `xml:"element" json:"-"`
	Name        string      `xml:"name,attr" json:"name"`
	Required    string      `xml:"required,attr" json:"required"`
	Type        string      `xml:"type,attr,omitempty" json:"type,omitempty"`
	Default     string      `xml:"default,attr,omitempty" json:"default,omitempty"`
	Description string      `xml:"description" json:"description"`
	Attributes  []Attribute `xml:"attribute" json:"attributes,omitempty"`
	Elements    []Element   `xml:"element" json:"elements,omitempty"`
	Includes    []Include   `xml:"include" json:"includes,omitempty"`
}

// Attribute is an XML attribute definition.
type Attribute struct {
	XMLName     xml.Name `xml:"attribute" json:"-"`
	Name        string   `xml:"name,attr" json:"name"`
	Required    string   `xml:"required,attr" json:"required"`
	Type        string   `xml:"type,attr,omitempty" json:"type,omitempty"`
	Default     string   `xml:"default,attr,omitempty" json:"default,omitempty"`
	Description string   `xml:"description" json:"description"`
}

// Include is a reference to another schema file.
type Include struct {
	XMLName  xml.Name `xml:"include" json:"-"`
	Filename string   `xml:"filename,attr" json:"filename"`
	Required string   `xml:"required,attr" json:"required"`
}

// Registry is the in-memory schema index.
type Registry struct {
	mu       sync.RWMutex
	versions map[string]map[string]*Element // version -> element name -> Element
	raw      map[string]map[string]string   // version -> filename -> raw .sdf body
}

// defaultReg is the package-level registry populated from the embedded
// schema FS on first use. Tests can bypass it via NewRegistry.
var (
	defaultOnce sync.Once
	defaultReg  *Registry
	defaultErr  error
)

// Default returns the process-wide registry, loading it on first call.
func Default() (*Registry, error) {
	defaultOnce.Do(func() {
		defaultReg = NewRegistry()
		defaultErr = defaultReg.LoadEmbedded()
	})
	return defaultReg, defaultErr
}

// MustDefault returns Default() and panics on failure. Convenient for
// startup wire-up where the only reasonable response to a load failure is
// to not boot the server.
func MustDefault() *Registry {
	r, err := Default()
	if err != nil {
		panic(fmt.Sprintf("sdf: failed to load embedded schemas: %v", err))
	}
	return r
}

// NewRegistry returns an empty, non-populated registry.
func NewRegistry() *Registry {
	return &Registry{
		versions: make(map[string]map[string]*Element),
		raw:      make(map[string]map[string]string),
	}
}

// LoadEmbedded walks the embedded schemas/ tree and populates the registry.
func (r *Registry) LoadEmbedded() error {
	return fs.WalkDir(schemaFS, "schemas", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".sdf") {
			return nil
		}
		// Path shape: schemas/<version>/<name>.sdf
		parts := strings.Split(p, "/")
		if len(parts) != 3 {
			return nil
		}
		version := parts[1]
		data, err := schemaFS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		r.addFile(version, d.Name(), data)
		return nil
	})
}

// addFile records the raw bytes and attempts an XML parse. A parse
// failure does not fail the load (some older schemas don't parse with the
// simplified Element shape); the raw text is still available via Raw.
func (r *Registry) addFile(version, filename string, data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.versions[version]; !ok {
		r.versions[version] = make(map[string]*Element)
		r.raw[version] = make(map[string]string)
	}
	r.raw[version][filename] = string(data)
	elem := &Element{}
	if err := xml.Unmarshal(data, elem); err != nil {
		return
	}
	if elem.Name == "" {
		elem.Name = strings.TrimSuffix(filename, ".sdf")
	}
	r.versions[version][elem.Name] = elem
}

// Versions returns the list of loaded SDF versions, sorted.
func (r *Registry) Versions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	vs := make([]string, 0, len(r.versions))
	for v := range r.versions {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	return vs
}

// Get returns the parsed Element for a name in a version.
func (r *Registry) Get(version, name string) (*Element, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.versions[version]
	if !ok {
		return nil, false
	}
	e, ok := v[name]
	return e, ok
}

// ListElements returns all known element names for a version, sorted.
func (r *Registry) ListElements(version string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.versions[version]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(v))
	for n := range v {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ChildNames returns direct child element names for one element,
// resolving one level of <include>.
func (r *Registry) ChildNames(version, name string) []string {
	e, ok := r.Get(version, name)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	out := []string{}
	add := func(n string) {
		if n == "" {
			return
		}
		if _, already := seen[n]; already {
			return
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	for _, c := range e.Elements {
		add(c.Name)
	}
	for _, inc := range e.Includes {
		add(strings.TrimSuffix(inc.Filename, ".sdf"))
	}
	sort.Strings(out)
	return out
}

// FullSchema returns every schema file for a version concatenated
// (filename header + body). This is the payload the AI Builder injects
// into its system prompt.
func (r *Registry) FullSchema(version string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	files, ok := r.raw[version]
	if !ok || len(files) == 0 {
		return "", false
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- SDF %s schema bundle -->\n", version)
	for _, n := range names {
		fmt.Fprintf(&b, "<!-- %s -->\n%s\n", path.Join(version, n), files[n])
	}
	return b.String(), true
}
