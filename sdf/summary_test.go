package sdf

import "testing"

func TestSummarize(t *testing.T) {
	s, err := Summarize(goodWorld)
	if err != nil {
		t.Fatal(err)
	}
	if s.Version != "1.9" || s.World != "demo" || len(s.Plugins) != 1 || len(s.Entities) != 2 {
		t.Fatalf("summary %+v", s)
	}
	inc, cart := s.Entities[0], s.Entities[1]
	if inc.Kind != "include" || inc.Name != "table" || inc.Pose != "1 0 0 0 0 0" || inc.URI == "" || inc.Line == 0 {
		t.Errorf("include %+v", inc)
	}
	if cart.Kind != "model" || cart.Links != 2 || cart.Joints != 1 || cart.Shape != "" {
		t.Errorf("model %+v", cart)
	}
}

func TestSummarizeShape(t *testing.T) {
	s, err := Summarize(`<sdf version="1.9"><model name="wall"><static>true</static><link name="l">
	<collision name="c"><geometry><box><size>4 0.2 3</size></box></geometry></collision></link></model></sdf>`)
	if err != nil {
		t.Fatal(err)
	}
	if e := s.Entities[0]; e.Shape != "box 4 0.2 3" || !e.Static {
		t.Fatalf("entity %+v", e)
	}
}

func TestSummarizeRejectsBadXML(t *testing.T) {
	if _, err := Summarize("<sdf"); err == nil {
		t.Fatal("expected error")
	}
}
