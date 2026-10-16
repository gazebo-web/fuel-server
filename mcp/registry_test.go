package mcp

import "testing"

func TestRegisterRejectsInvalidNames(t *testing.T) {
	for _, name := range []string{"", "fuel.search_models", "Fuel_Search", "has space", "dash-name"} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("Register(%q) did not panic", name)
				}
			}()
			NewRegistry().Register(Tool{Spec: ToolSpec{Name: name}})
		})
	}
}

func TestRegisterAcceptsUnderscoreNames(t *testing.T) {
	r := NewRegistry()
	r.Register(Tool{Spec: ToolSpec{Name: "fuel_search_models"}})
	if _, ok := r.Get("fuel_search_models"); !ok {
		t.Fatal("tool not registered")
	}
}
