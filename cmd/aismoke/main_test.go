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
		"event: usage\ndata: {\"usage\":{\"input_tokens\":10,\"cache_read_tokens\":90,\"output_tokens\":7,\"thinking_tokens\":5}}\n\n" +
		"event: usage\ndata: {\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}\n\n" +
		"event: done\ndata: {}\n\n" +
		"event: text\ndata: {\"text\":\"after done\"}\n\n"
	tl := tally{}
	if err := readSSE(strings.NewReader(stream), tl.add); err != nil {
		t.Fatal(err)
	}
	if tl.usage != (Usage{Calls: 2, Input: 11, CacheRead: 90, Output: 9, Thinking: 5}) {
		t.Errorf("usage %+v", tl.usage)
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
	score(Case{MinEntities: 2, WantInclude: true}, world, nil, &res)
	if !res.Passed || res.Entities != 2 || res.Includes != 1 {
		t.Fatalf("want pass: %+v", res)
	}

	res = Result{Writes: 0}
	score(Case{MinEntities: 3, WantJoints: 1}, `<sdf version="1.9"><world name="w"><model name="m"><static>maybe</static></model></world></sdf>`, nil, &res)
	got := strings.Join(res.Failures, "|")
	for _, want := range []string{"validation errors", "no edits", "entities", "joints"} {
		if !strings.Contains(got, want) {
			t.Errorf("failures %q missing %q", got, want)
		}
	}
}

func TestScoreProjectModels(t *testing.T) {
	world := `<sdf version="1.9"><world name="w">
	  <include><uri>model://robot</uri><name>r1</name></include>
	  <include><uri>model://robot</uri><name>r2</name></include>
	  <include><uri>model://loop</uri></include>
	</world></sdf>`
	models := map[string]string{
		"robot": `<sdf version="1.9"><model name="robot">
		  <link name="a"/><link name="b"/><link name="c"/>
		  <joint name="j1" type="revolute"><parent>a</parent><child>b</child></joint>
		  <joint name="j2" type="revolute"><parent>a</parent><child>c</child></joint>
		  <include><uri>https://fuel.gazebosim.org/1.0/OpenRobotics/models/Camera</uri></include>
		</model></sdf>`,
		"loop": `<sdf version="1.9"><model name="loop"><include><uri>model://loop</uri></include></model></sdf>`,
	}
	load := func(name string) (string, bool) { src, ok := models[name]; return src, ok }
	res := Result{Writes: 1}
	score(Case{MinEntities: 3, WantJoints: 4, WantInclude: true}, world, load, &res)
	if !res.Passed || res.Entities != 3 || res.Joints != 4 || res.Includes != 2 || res.Models != 4 {
		t.Fatalf("want pass with 3 entities, 4 joints, 2 Fuel includes, 4 model includes: %+v", res)
	}
}

func TestTallyCountsCallsPerTool(t *testing.T) {
	stream := "event: tool_call\ndata: {\"tool\":\"model_generate\"}\n\n" +
		"event: tool_call\ndata: {\"tool\":\"mesh_api_reference\"}\n\n" +
		"event: tool_call\ndata: {\"tool\":\"model_generate\"}\n\n" +
		"event: done\ndata: {}\n\n"
	tl := tally{calls: map[string]int{}}
	if err := readSSE(strings.NewReader(stream), tl.add); err != nil {
		t.Fatal(err)
	}
	if tl.calls["model_generate"] != 2 || tl.calls["mesh_api_reference"] != 1 || tl.toolCalls != 3 {
		t.Fatalf("%+v", tl.calls)
	}
}
