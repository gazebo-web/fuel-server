package sdf

import (
	"errors"
	"strings"
)

// Entity is one top-level thing in a scene: a model, include, light,
// actor or frame. Nested models appear in Children.
type Entity struct {
	Kind   string `json:"kind"`
	Name   string `json:"name,omitempty"`
	Line   int    `json:"line"`
	Pose   string `json:"pose,omitempty"`
	URI    string `json:"uri,omitempty"`
	Static bool   `json:"static,omitempty"`
	Links  int    `json:"links,omitempty"`
	Joints int    `json:"joints,omitempty"`
	// Shape describes simple primitive models, e.g. "box 2 0.2 3".
	Shape    string   `json:"shape,omitempty"`
	Children []Entity `json:"children,omitempty"`
	// BBox is an included model's size, filled in by callers that know it.
	BBox any `json:"bbox,omitempty"`
}

// Summary is a compact view of an SDF document, much smaller than the
// document itself. Agents read it to orient before editing (design §5.2).
type Summary struct {
	Version  string   `json:"version"`
	World    string   `json:"world,omitempty"`
	Plugins  []string `json:"plugins,omitempty"`
	Entities []Entity `json:"entities"`
}

// Summarize parses an SDF document into a Summary. Line numbers point
// back into the document so the agent can read or edit around them.
func Summarize(input string) (Summary, error) {
	root, perr := parseTree(input)
	if perr != nil {
		return Summary{}, errors.New("malformed XML: " + perr.msg)
	}
	if root.name != "sdf" {
		return Summary{}, errors.New("root element must be <sdf>")
	}
	s := Summary{Version: root.attr("version")}
	scope := root
	if w := root.child("world"); w != nil {
		s.World = w.attr("name")
		scope = w
		for _, c := range w.children {
			if c.name == "plugin" {
				s.Plugins = append(s.Plugins, c.attr("filename"))
			}
		}
	}
	s.Entities = entities(scope)
	return s, nil
}

func entities(scope *node) []Entity {
	var out []Entity
	for _, c := range scope.children {
		switch c.name {
		case "model", "include", "light", "actor", "frame":
		default:
			continue
		}
		e := Entity{Kind: c.name, Name: c.attr("name"), Line: c.line}
		if p := c.child("pose"); p != nil {
			e.Pose = p.text
		}
		if st := c.child("static"); st != nil {
			e.Static = st.text == "true" || st.text == "1"
		}
		switch c.name {
		case "include":
			if n := c.child("name"); n != nil {
				e.Name = n.text
			}
			if u := c.child("uri"); u != nil {
				e.URI = u.text
			}
		case "model":
			for _, cc := range c.children {
				switch cc.name {
				case "link":
					e.Links++
				case "joint":
					e.Joints++
				}
			}
			if e.Links == 1 {
				e.Shape = shapeOf(c.child("link"))
			}
			e.Children = entities(c)
		}
		out = append(out, e)
	}
	return out
}

// shapeOf describes a link whose first collision (or visual) is a simple
// primitive, e.g. "box 1 2 3" or "cylinder r=0.5 l=2".
func shapeOf(link *node) string {
	var geom *node
	for _, kind := range []string{"collision", "visual"} {
		if c := link.child(kind); c != nil {
			if geom = c.child("geometry"); geom != nil {
				break
			}
		}
	}
	if geom == nil || len(geom.children) == 0 {
		return ""
	}
	g := geom.children[0]
	field := func(name string) string {
		if c := g.child(name); c != nil {
			return c.text
		}
		return ""
	}
	switch g.name {
	case "box":
		return "box " + field("size")
	case "sphere":
		return "sphere r=" + field("radius")
	case "cylinder", "capsule":
		return g.name + " r=" + field("radius") + " l=" + field("length")
	case "plane":
		return strings.TrimSpace("plane " + field("size"))
	case "mesh":
		return "mesh " + field("uri")
	}
	return g.name
}
