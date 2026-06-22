package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

// Ordered lists must number items 1., 2., 3. — not repeat "1." for every item
// (regression for the static-bullet bug).
func TestRenderADFOrderedList(t *testing.T) {
	doc := `{"type":"doc","content":[
		{"type":"orderedList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"first"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"second"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"third"}]}]}
		]}
	]}`
	got := renderADF(json.RawMessage(doc))
	for _, want := range []string{"1. first", "2. second", "3. third"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Count(got, "1. ") != 1 {
		t.Errorf("ordered list should number once each, got:\n%s", got)
	}
}

func TestRenderADFNodeTypes(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string
	}{
		{"heading", `{"type":"doc","content":[{"type":"heading","content":[{"type":"text","text":"Title"}]}]}`, "## Title"},
		{"codeBlock", `{"type":"doc","content":[{"type":"codeBlock","content":[{"type":"text","text":"x := 1"}]}]}`, "```"},
		{"blockquote", `{"type":"doc","content":[{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"quoted"}]}]}]}`, "> quoted"},
		{"rule", `{"type":"doc","content":[{"type":"rule"}]}`, "---"},
		{"mention", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"text":"@dev"}}]}]}`, "@dev"},
		{"emoji-text", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"emoji","attrs":{"text":"🚀"}}]}]}`, "🚀"},
		{"emoji-shortname", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"emoji","attrs":{"shortName":":rocket:"}}]}]}`, ":rocket:"},
		{"inlineCard", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"inlineCard","attrs":{"url":"https://x.test"}}]}]}`, "https://x.test"},
		{"unknown-node-keeps-text", `{"type":"doc","content":[{"type":"weirdNode","content":[{"type":"text","text":"survives"}]}]}`, "survives"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderADF(json.RawMessage(c.doc))
			if !strings.Contains(got, c.want) {
				t.Errorf("want %q in:\n%s", c.want, got)
			}
		})
	}
}

func TestApplyMarks(t *testing.T) {
	cases := []struct {
		doc  string
		want string
	}{
		{`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"i","marks":[{"type":"em"}]}]}]}`, "*i*"},
		{`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"c","marks":[{"type":"code"}]}]}]}`, "`c`"},
		{`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"s","marks":[{"type":"strike"}]}]}]}`, "~~s~~"},
		{`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"go","marks":[{"type":"link","attrs":{"href":"https://go.dev"}}]}]}]}`, "[go](https://go.dev)"},
	}
	for _, c := range cases {
		if got := renderADF(json.RawMessage(c.doc)); !strings.Contains(got, c.want) {
			t.Errorf("want %q in %q", c.want, got)
		}
	}
}
