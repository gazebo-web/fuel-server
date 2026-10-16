package main

import (
	"strings"
	"testing"
)

func TestEmbeddedCorpusLoads(t *testing.T) {
	cases, err := loadCorpus("", "")
	if err != nil || len(cases) < 5 {
		t.Fatalf("corpus: %d cases, %v", len(cases), err)
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || c.Prompt == "" || seen[c.ID] {
			t.Errorf("bad or duplicate case %+v", c)
		}
		seen[c.ID] = true
	}
	only, _ := loadCorpus("", "kitchen, parking-lot")
	if len(only) != 2 {
		t.Fatalf("-only selected %d cases", len(only))
	}
}

func TestReadSSE(t *testing.T) {
	stream := "event: text\ndata: {\"type\":\"text\",\"text\":\"Building\"}\n\n" +
		"event: tool_call\ndata: {\"tool\":\"project_write_file\"}\n\n" +
		"event: project_changed\ndata: {\"path\":\"world.sdf\",\"rev\":\"abc\"}\n\n" +
		"event: done\ndata: {}\n\n" +
		"event: text\ndata: {\"text\":\"after done\"}\n\n"
	tl := tally{}
	if err := readSSE(strings.NewReader(stream), tl.add); err != nil {
		t.Fatal(err)
	}
	if tl.writes != 1 || tl.toolCalls != 1 || tl.reply.String() != "Building" {
		t.Fatalf("tally %+v reply %q", tl, tl.reply.String())
	}
}

func TestScore(t *testing.T) {
	world := `<sdf version="1.9"><world name="w">
	  <include><uri>https://fuel.gazebosim.org/1.0/OpenRobotics/models/Table</uri><name>t</name></include>
	  <model name="box"><link name="l"/></model>
	</world></sdf>`
	res := Result{Writes: 2}
	score(Case{MinEntities: 2, WantInclude: true}, world, &res)
	if !res.Passed || res.Entities != 2 || res.Includes != 1 {
		t.Fatalf("want pass: %+v", res)
	}

	res = Result{Writes: 0}
	score(Case{MinEntities: 3, WantJoints: 1}, `<sdf version="1.9"><world name="w"><model name="m"><static>maybe</static></model></world></sdf>`, &res)
	got := strings.Join(res.Failures, "|")
	for _, want := range []string{"validation errors", "no edits", "entities", "joints"} {
		if !strings.Contains(got, want) {
			t.Errorf("failures %q missing %q", got, want)
		}
	}
}
