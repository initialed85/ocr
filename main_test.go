package main

import (
	"strings"
	"testing"
)

func TestTsvToJSON(t *testing.T) {
	tsv := strings.Join([]string{
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext",
		"4\t1\t1\t1\t1\t1\t10\t20\t30\t40\t96.5\tHello",
		"4\t1\t1\t1\t1\t2\t50\t20\t30\t40\t88.1\tworld",
		"4\t1\t2\t1\t1\t1\t10\t80\t60\t40\t41.2\tmaybe",
	}, "\n")

	got := tsvToJSON([]byte(tsv))
	text, _ := got["text"].(string)
	if text != "Hello world maybe" {
		t.Fatalf("text = %q, want %q", text, "Hello world maybe")
	}

	words, _ := got["words"].([]map[string]any)
	if len(words) != 3 {
		t.Fatalf("words = %d, want 3", len(words))
	}
	if words[0]["text"] != "Hello" || words[0]["conf"].(float64) != 96.5 {
		t.Fatalf("first word = %+v", words[0])
	}
	bbox, _ := words[1]["bbox"].([]int)
	if len(bbox) != 4 || bbox[0] != 50 {
		t.Fatalf("second bbox = %v", bbox)
	}
}

func TestTsvToJSONEmpty(t *testing.T) {
	got := tsvToJSON([]byte(""))
	if got["text"] != "" {
		t.Fatalf("text = %q, want empty", got["text"])
	}
	if len(got["words"].([]any)) != 0 {
		t.Fatalf("words should be empty")
	}
}
