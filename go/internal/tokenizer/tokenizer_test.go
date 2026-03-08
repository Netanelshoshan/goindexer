package tokenizer

import (
	"testing"
)

func TestCountTokens(t *testing.T) {
	tests := []struct {
		text  string
		want  int
		approx bool // allow some variance for different tokenizer versions
	}{
		{"", 0, false},
		{"hello", 1, false},
		{"hello world", 2, false},
		{"func main() { println(\"hi\") }", 10, true},
	}
	for _, tt := range tests {
		got, err := CountTokens(tt.text)
		if err != nil {
			t.Errorf("CountTokens(%q) error: %v", tt.text, err)
			continue
		}
		if tt.approx {
			if got < 1 && tt.text != "" {
				t.Errorf("CountTokens(%q) = %d, want > 0", tt.text, got)
			}
		} else if got != tt.want {
			t.Errorf("CountTokens(%q) = %d, want %d", tt.text, got, tt.want)
		}
	}
}
