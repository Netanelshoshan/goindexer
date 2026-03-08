package chunk

import (
	"testing"
)

func TestCodeChunk_ID(t *testing.T) {
	c := &CodeChunk{
		FilePath:  "src/main.go",
		StartLine: 10,
		EndLine:   25,
	}
	id := c.ID()
	if id != "src/main.go:10:25" {
		t.Errorf("ID() = %q, want src/main.go:10:25", id)
	}
}

func TestCodeChunk_ID_Zero(t *testing.T) {
	c := &CodeChunk{
		FilePath:  "x",
		StartLine: 0,
		EndLine:   0,
	}
	id := c.ID()
	if id != "x:0:0" {
		t.Errorf("ID() = %q, want x:0:0", id)
	}
}
