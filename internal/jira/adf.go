package jira

import (
	"encoding/json"
	"fmt"
	"strings"
)

// adfNode is a node in an Atlassian Document Format tree.
type adfNode struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Content []adfNode       `json:"content"`
	Marks   []adfMark       `json:"marks"`
	Attrs   json.RawMessage `json:"attrs"`
}

type adfMark struct {
	Type  string `json:"type"`
	Attrs struct {
		Href string `json:"href"`
	} `json:"attrs"`
}

// renderADF converts an ADF document (raw JSON) to markdown-ish plain text.
// Anything it doesn't recognize is traversed for its text content, so it
// degrades gracefully rather than dropping data.
func renderADF(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// A plain string description (older APIs / non-ADF) decodes directly.
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var doc adfNode
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	var b strings.Builder
	renderNodes(&b, doc.Content, "")
	return strings.TrimSpace(collapseBlankLines(b.String()))
}

func renderNodes(b *strings.Builder, nodes []adfNode, listPrefix string) {
	for i := range nodes {
		renderNode(b, &nodes[i], listPrefix)
	}
}

func renderNode(b *strings.Builder, n *adfNode, listPrefix string) {
	switch n.Type {
	case "text":
		b.WriteString(applyMarks(n.Text, n.Marks))
	case "hardBreak":
		b.WriteString("\n")
	case "paragraph":
		renderNodes(b, n.Content, listPrefix)
		b.WriteString("\n\n")
	case "heading":
		b.WriteString("## ")
		renderNodes(b, n.Content, listPrefix)
		b.WriteString("\n\n")
	case "bulletList":
		renderList(b, n.Content, false)
	case "orderedList":
		renderList(b, n.Content, true)
	case "listItem":
		renderNodes(b, n.Content, listPrefix)
	case "codeBlock":
		b.WriteString("```\n")
		renderNodes(b, n.Content, "")
		b.WriteString("\n```\n\n")
	case "blockquote":
		var inner strings.Builder
		renderNodes(&inner, n.Content, listPrefix)
		for _, line := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
			b.WriteString("> " + line + "\n")
		}
		b.WriteString("\n")
	case "rule":
		b.WriteString("\n---\n\n")
	case "mention":
		b.WriteString(attrText(n.Attrs, "text"))
	case "emoji":
		if t := attrText(n.Attrs, "text"); t != "" {
			b.WriteString(t)
		} else {
			b.WriteString(attrText(n.Attrs, "shortName"))
		}
	case "inlineCard", "blockCard":
		if u := attrText(n.Attrs, "url"); u != "" {
			b.WriteString(u)
		}
	case "mediaSingle", "mediaGroup", "media":
		b.WriteString("[media]")
		renderNodes(b, n.Content, listPrefix)
	default:
		// Unknown node: recurse for any text content so nothing is silently lost.
		renderNodes(b, n.Content, listPrefix)
	}
}

// attrText pulls a string attribute (e.g. "text", "url", "shortName") from an
// ADF node's attrs object.
func attrText(raw json.RawMessage, keys ...string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func renderList(b *strings.Builder, items []adfNode, ordered bool) {
	n := 0
	for i := range items {
		var inner strings.Builder
		renderNode(&inner, &items[i], "")
		text := strings.TrimSpace(inner.String())
		if text == "" {
			continue
		}
		n++
		bullet := "- "
		if ordered {
			bullet = fmt.Sprintf("%d. ", n)
		}
		// indent continuation lines of multi-line items
		lines := strings.Split(text, "\n")
		b.WriteString(bullet + lines[0] + "\n")
		for _, l := range lines[1:] {
			b.WriteString("  " + l + "\n")
		}
	}
	b.WriteString("\n")
}

func applyMarks(text string, marks []adfMark) string {
	for _, m := range marks {
		switch m.Type {
		case "strong":
			text = "**" + text + "**"
		case "em":
			text = "*" + text + "*"
		case "code":
			text = "`" + text + "`"
		case "strike":
			text = "~~" + text + "~~"
		case "link":
			if m.Attrs.Href != "" {
				text = "[" + text + "](" + m.Attrs.Href + ")"
			}
		}
	}
	return text
}

func collapseBlankLines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}
