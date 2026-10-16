package sdf

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// ValidationError is one problem found during SDF validation.
type ValidationError struct {
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	// Path locates the element, e.g. /sdf/world[w]/model[box]/link[base].
	Path string `json:"path,omitempty"`
}

// ValidationResult is the outcome of validating an SDF string. Valid is
// false when there is at least one error; warnings never affect it.
type ValidationResult struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []ValidationError `json:"warnings,omitempty"`
}

// Summary renders the result as compact, model-friendly lines, e.g.
//
//	world.sdf:42 /sdf/world[w]/model[m]/link[l]/inertial: ixx+iyy < izz
//
// file prefixes every line; pass "" to omit it.
func (r ValidationResult) Summary(file string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d error(s), %d warning(s)", len(r.Errors), len(r.Warnings))
	write := func(kind string, items []ValidationError) {
		for _, e := range items {
			b.WriteString("\n")
			b.WriteString(kind)
			b.WriteString(" ")
			if file != "" {
				b.WriteString(file)
				if e.Line > 0 {
					fmt.Fprintf(&b, ":%d", e.Line)
				}
				b.WriteString(" ")
			} else if e.Line > 0 {
				fmt.Fprintf(&b, "line %d ", e.Line)
			}
			if e.Path != "" {
				b.WriteString(e.Path)
				b.WriteString(": ")
			}
			b.WriteString(e.Message)
		}
	}
	write("error", r.Errors)
	write("warning", r.Warnings)
	return b.String()
}

// Options configures ValidateWithOptions.
type Options struct {
	// Registry supplies the schemas. Nil uses the embedded default.
	Registry *Registry
	// ResolveURI, when set, is called for every <include><uri>. A non-nil
	// error is reported as a validation error on that element.
	ResolveURI func(uri string) error
}

// Validate checks an SDF document against the embedded schemas. See
// ValidateWithOptions.
func Validate(input string) ValidationResult {
	return ValidateWithOptions(input, Options{})
}

// ValidateWithOptions checks an SDF document in layers (design §5.3):
//
//  1. XML parses; root is <sdf> with a version.
//  2. Elements, attributes and value types match the schema for that
//     version. Unknown elements and attributes are warnings (libsdformat
//     ignores them), everything else is an error.
//  3. Semantics: unique sibling names, joint parent/child resolve,
//     include URIs resolve (when ResolveURI is set).
//  4. Physics: positive mass, physically valid inertia.
//
// It is not a full libsdformat conformance check (design Q-4).
func ValidateWithOptions(input string, opts Options) ValidationResult {
	v := &validator{opts: opts}
	root, err := parseTree(input)
	if err != nil {
		v.errorAt(nil, err.line, "malformed XML: %s", err.msg)
		return v.result()
	}
	if root.name != "sdf" {
		v.errorAt(root, 0, "root element must be <sdf>, got <%s>", root.name)
		return v.result()
	}
	version := root.attr("version")
	if version == "" {
		v.errorAt(root, 0, "<sdf> must have a version attribute")
	}
	if !root.hasChild("world", "model", "actor", "light") {
		v.errorAt(root, 0, "<sdf> must contain at least one of: world, model, actor, light")
	}

	reg := opts.Registry
	if reg == nil {
		var regErr error
		if reg, regErr = Default(); regErr != nil {
			reg = nil
		}
	}
	if version != "" && reg != nil {
		if schema, ok := reg.Get(version, "sdf"); ok {
			v.reg, v.version = reg, version
			v.checkSchema(root, schema)
		} else {
			v.warnAt(root, 0, "schema checks skipped: SDF %s is not bundled (have %s)",
				version, strings.Join(reg.Versions(), ", "))
		}
	}
	v.checkSemantics(root)
	return v.result()
}

// -- document tree ----------------------------------------------------------

type node struct {
	name     string
	attrs    []xml.Attr
	text     string
	line     int
	parent   *node
	children []*node
}

func (n *node) attr(name string) string {
	for _, a := range n.attrs {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (n *node) child(name string) *node {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

func (n *node) hasChild(names ...string) bool {
	for _, c := range n.children {
		for _, want := range names {
			if c.name == want {
				return true
			}
		}
	}
	return false
}

// path renders /sdf/world[w]/model[m], using the name attribute (or an
// include's <name>) as the bracketed label when present.
func (n *node) path() string {
	if n == nil {
		return ""
	}
	label := n.name
	name := n.attr("name")
	if name == "" && n.name == "include" {
		if c := n.child("name"); c != nil {
			name = c.text
		}
	}
	if name != "" {
		label += "[" + name + "]"
	}
	return n.parent.path() + "/" + label
}

type parseError struct {
	line int
	msg  string
}

func parseTree(input string) (*node, *parseError) {
	dec := xml.NewDecoder(strings.NewReader(tolerateTinyXML(input)))
	var root, cur *node
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var se *xml.SyntaxError
			if errors.As(err, &se) {
				return nil, &parseError{line: se.Line, msg: se.Msg}
			}
			line, _ := dec.InputPos()
			return nil, &parseError{line: line, msg: err.Error()}
		}
		switch t := tok.(type) {
		case xml.StartElement:
			line, _ := dec.InputPos()
			n := &node{name: elementName(t.Name), attrs: t.Attr, line: line, parent: cur}
			if cur == nil {
				if root != nil {
					return nil, &parseError{line: line, msg: "multiple root elements"}
				}
				root = n
			} else {
				cur.children = append(cur.children, n)
			}
			cur = n
			text.Reset()
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			if cur != nil {
				if len(cur.children) == 0 {
					cur.text = strings.TrimSpace(text.String())
				}
				cur = cur.parent
			}
			text.Reset()
		}
	}
	if root == nil {
		return nil, &parseError{msg: "document is empty"}
	}
	return root, nil
}

// tolerateTinyXML rewrites two constructs that libsdformat's XML parser
// (tinyxml2) accepts but encoding/xml rejects, so real Gazebo files don't
// fail as "malformed":
//   - "--" inside comments
//   - "::" in element names (<gz::system_priority>), rewritten to ":_" so
//     the element is still treated as namespaced and skipped
//
// Replacements keep the same length, so reported line numbers still match
// the original document.
func tolerateTinyXML(s string) string {
	if !strings.Contains(s, "--") && !strings.Contains(s, "::") {
		return s
	}
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		if b[i] != '<' {
			continue
		}
		if strings.HasPrefix(s[i:], "<!--") {
			end := strings.Index(s[i+4:], "-->")
			if end < 0 {
				return string(b)
			}
			for j := i + 4; j < i+4+end-1; j++ {
				if b[j] == '-' && b[j+1] == '-' {
					b[j], b[j+1] = ' ', ' '
				}
			}
			i += 4 + end + 2
			continue
		}
		// Tag name: after "<" or "</" up to whitespace, "/" or ">".
		j := i + 1
		if j < len(b) && b[j] == '/' {
			j++
		}
		for ; j < len(b) && !strings.ContainsRune(" \t\r\n/>", rune(b[j])); j++ {
			if b[j] == ':' && j+1 < len(b) && b[j+1] == ':' {
				b[j+1] = '_'
			}
		}
	}
	return string(b)
}

// elementName keeps a namespace prefix visible ("gz:foo") so custom,
// namespaced elements can be recognized and skipped.
func elementName(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

// -- validator --------------------------------------------------------------

type validator struct {
	opts     Options
	reg      *Registry
	version  string
	errs     []ValidationError
	warnings []ValidationError
}

func (v *validator) errorAt(n *node, line int, format string, args ...any) {
	v.errs = append(v.errs, issue(n, line, format, args...))
}

func (v *validator) warnAt(n *node, line int, format string, args ...any) {
	v.warnings = append(v.warnings, issue(n, line, format, args...))
}

func issue(n *node, line int, format string, args ...any) ValidationError {
	if n != nil && line == 0 {
		line = n.line
	}
	return ValidationError{Message: fmt.Sprintf(format, args...), Line: line, Path: n.path()}
}

func (v *validator) result() ValidationResult {
	return ValidationResult{Valid: len(v.errs) == 0, Errors: v.errs, Warnings: v.warnings}
}

// schemaChildren returns the element's allowed children keyed by name,
// resolving <include> files, and whether it accepts arbitrary children
// (the plugin's copy_data element).
func (v *validator) schemaChildren(e *Element) (map[string]*Element, bool) {
	out := map[string]*Element{}
	anyChild := false
	for i := range e.Elements {
		c := &e.Elements[i]
		if c.Name == "" {
			anyChild = true
			continue
		}
		if c.Ref != "" {
			// e.g. a nested <model ref="model">: reuse the full definition.
			if re, ok := v.reg.ElementForFile(v.version, c.Ref+".sdf"); ok {
				cp := *re
				cp.Required = c.Required
				c = &cp
			}
		}
		out[c.Name] = c
	}
	for _, inc := range e.Includes {
		if ie, ok := v.reg.ElementForFile(v.version, inc.Filename); ok {
			// The include's required flag overrides the file's own.
			cp := *ie
			cp.Required = inc.Required
			out[cp.Name] = &cp
		}
	}
	return out, anyChild
}

func (v *validator) checkSchema(n *node, schema *Element) {
	// Attributes.
	known := map[string]Attribute{}
	for _, a := range schema.Attributes {
		known[a.Name] = a
	}
	for _, a := range n.attrs {
		if a.Name.Space != "" || a.Name.Local == "xmlns" {
			continue
		}
		def, ok := known[a.Name.Local]
		if !ok {
			v.warnAt(n, 0, "unknown attribute %q on <%s> (ignored by Gazebo; check spelling)",
				a.Name.Local, n.name)
			continue
		}
		if msg := checkValue(def.Type, a.Value); msg != "" {
			v.errorAt(n, 0, "attribute %s=%q: %s", a.Name.Local, a.Value, msg)
		}
	}
	for _, a := range schema.Attributes {
		if a.Required == "1" && n.attr(a.Name) == "" {
			v.errorAt(n, 0, "<%s> is missing required attribute %q", n.name, a.Name)
		}
	}

	// Leaf value.
	if schema.Type != "" && len(n.children) == 0 && n.text != "" {
		if msg := checkValue(schema.Type, n.text); msg != "" {
			v.errorAt(n, 0, "<%s> value %q: %s", n.name, n.text, msg)
		}
	}

	// Children.
	allowed, anyChild := v.schemaChildren(schema)
	counts := map[string]int{}
	for _, c := range n.children {
		counts[c.name]++
		if strings.Contains(c.name, ":") {
			continue // custom namespaced element
		}
		cs, ok := allowed[c.name]
		if !ok {
			if !anyChild {
				v.warnAt(c, 0, "unknown element <%s> inside <%s> (ignored by Gazebo; check spelling and placement)",
					c.name, n.name)
			}
			continue
		}
		v.checkSchema(c, cs)
	}
	names := make([]string, 0, len(allowed))
	for name := range allowed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cs := allowed[name]
		if counts[name] > 1 && (cs.Required == "0" || cs.Required == "1") {
			v.warnAt(n, 0, "<%s> appears %d times inside <%s>; only one is used", name, counts[name], n.name)
		}
		// Required children with a real default are filled in by
		// libsdformat; only a "__default__" placeholder must be given.
		if counts[name] == 0 && (cs.Required == "1" || cs.Required == "+") && cs.Default == "__default__" {
			v.errorAt(n, 0, "<%s> is missing required <%s>", n.name, name)
		}
	}
}

// checkValue returns "" when s parses as the sdformat type t.
func checkValue(t, s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	fields := strings.Fields(s)
	floats := func(min, max int) string {
		if len(fields) < min || len(fields) > max {
			if min == max {
				return fmt.Sprintf("expected %d numbers, got %d", min, len(fields))
			}
			return fmt.Sprintf("expected %d to %d numbers, got %d", min, max, len(fields))
		}
		for _, f := range fields {
			x, err := strconv.ParseFloat(f, 64)
			if err != nil || math.IsNaN(x) {
				return fmt.Sprintf("%q is not a number", f)
			}
		}
		return ""
	}
	switch t {
	case "bool":
		switch strings.ToLower(s) {
		case "true", "false", "1", "0":
			return ""
		}
		return "expected true or false"
	case "int":
		// Base 0 accepts hex (0xAB), as libsdformat does.
		if _, err := strconv.ParseInt(s, 0, 64); err != nil {
			return "expected an integer"
		}
	case "unsigned int":
		if _, err := strconv.ParseUint(s, 0, 64); err != nil {
			return "expected a non-negative integer"
		}
	case "double", "float":
		return floats(1, 1)
	case "vector2d":
		return floats(2, 2)
	case "vector2i":
		for _, f := range fields {
			if _, err := strconv.ParseInt(f, 10, 64); err != nil {
				return "expected 2 integers"
			}
		}
		if len(fields) != 2 {
			return "expected 2 integers"
		}
	case "vector3":
		return floats(3, 3)
	case "quaternion":
		return floats(4, 4)
	case "pose":
		// x y z roll pitch yaw, or x y z qw qx qy qz.
		if len(fields) != 6 && len(fields) != 7 {
			return fmt.Sprintf("expected 6 numbers (x y z roll pitch yaw), got %d", len(fields))
		}
		return floats(6, 7)
	case "color":
		return floats(3, 4)
	case "time":
		return floats(1, 2)
	}
	return ""
}

// -- semantic and physical checks -------------------------------------------

// namedKinds are siblings whose names must be unique within their parent.
var namedKinds = map[string]bool{
	"model": true, "include": true, "actor": true, "light": true, "frame": true,
	"link": true, "joint": true, "collision": true, "visual": true, "sensor": true,
}

func (v *validator) checkSemantics(n *node) {
	seen := map[string]*node{}
	for _, c := range n.children {
		if !namedKinds[c.name] {
			continue
		}
		name := c.attr("name")
		if c.name == "include" {
			name = ""
			if nc := c.child("name"); nc != nil {
				name = nc.text
			}
		}
		if name == "" {
			continue
		}
		// Links, joints and frames share one namespace inside a model;
		// so do models, includes, actors and lights inside a world.
		key := scopeOf(c.name) + "/" + name
		if prev, dup := seen[key]; dup {
			v.errorAt(c, 0, "duplicate name %q (also used by <%s> at line %d)", name, prev.name, prev.line)
			continue
		}
		seen[key] = c
	}

	switch n.name {
	case "model":
		v.checkJoints(n)
	case "include":
		if u := n.child("uri"); u != nil && u.text != "" && v.opts.ResolveURI != nil {
			if err := v.opts.ResolveURI(u.text); err != nil {
				v.errorAt(u, 0, "cannot resolve %q: %v", u.text, err)
			}
		}
	case "inertial":
		v.checkInertial(n)
	}
	for _, c := range n.children {
		if !strings.Contains(c.name, ":") {
			v.checkSemantics(c) // custom namespaced subtrees are opaque
		}
	}
}

func scopeOf(kind string) string {
	switch kind {
	case "link", "joint", "frame", "model", "include", "actor", "light":
		return "frame"
	}
	return kind
}

// checkJoints verifies each joint's parent and child name something in the
// same model. Scoped references (a::b) are not resolved here.
func (v *validator) checkJoints(model *node) {
	names := map[string]bool{"world": true}
	for _, c := range model.children {
		switch c.name {
		case "link", "joint", "frame", "model":
			names[c.attr("name")] = true
		case "include":
			if nc := c.child("name"); nc != nil {
				names[nc.text] = true
			}
		}
	}
	for _, j := range model.children {
		if j.name != "joint" {
			continue
		}
		for _, field := range []string{"parent", "child"} {
			f := j.child(field)
			if f == nil {
				v.errorAt(j, 0, "joint %q is missing <%s>", j.attr("name"), field)
				continue
			}
			if f.text == "" || strings.Contains(f.text, "::") || names[f.text] {
				continue
			}
			v.errorAt(f, 0, "joint %q %s %q does not name a link, joint, frame or model in this model",
				j.attr("name"), field, f.text)
		}
	}
}

func (v *validator) checkInertial(n *node) {
	if m := n.child("mass"); m != nil && m.text != "" {
		if x, err := strconv.ParseFloat(m.text, 64); err == nil && x <= 0 {
			v.errorAt(m, 0, "mass must be positive, got %s", m.text)
		}
	}
	in := n.child("inertia")
	if in == nil {
		return
	}
	get := func(name string) (float64, bool) {
		c := in.child(name)
		if c == nil || c.text == "" {
			return 0, false
		}
		x, err := strconv.ParseFloat(c.text, 64)
		return x, err == nil
	}
	ixx, ok1 := get("ixx")
	iyy, ok2 := get("iyy")
	izz, ok3 := get("izz")
	if !ok1 || !ok2 || !ok3 {
		return
	}
	if ixx <= 0 || iyy <= 0 || izz <= 0 {
		v.errorAt(in, 0, "principal moments must be positive (ixx=%g iyy=%g izz=%g)", ixx, iyy, izz)
		return
	}
	// Triangle inequality on principal moments, with a small tolerance.
	tol := 1e-9 * (ixx + iyy + izz)
	switch {
	case ixx+iyy < izz-tol:
		v.errorAt(in, 0, "inertia violates ixx+iyy >= izz (%g + %g < %g)", ixx, iyy, izz)
	case iyy+izz < ixx-tol:
		v.errorAt(in, 0, "inertia violates iyy+izz >= ixx (%g + %g < %g)", iyy, izz, ixx)
	case ixx+izz < iyy-tol:
		v.errorAt(in, 0, "inertia violates ixx+izz >= iyy (%g + %g < %g)", ixx, izz, iyy)
	}
}
