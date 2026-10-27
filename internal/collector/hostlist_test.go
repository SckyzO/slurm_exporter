package collector

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandHostList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "plain name",
			input: "na1",
			want:  []string{"na1"},
		},
		{
			name:  "comma-separated list",
			input: "na1,nb2,nc3",
			want:  []string{"na1", "nb2", "nc3"},
		},
		{
			name:  "simple range",
			input: "n[1-3]",
			want:  []string{"n1", "n2", "n3"},
		},
		{
			name:  "range and singletons mixed",
			input: "n[1-3,7,9-10]",
			want:  []string{"n1", "n2", "n3", "n7", "n9", "n10"},
		},
		{
			name:  "zero padding is preserved",
			input: "n[01-03]",
			want:  []string{"n01", "n02", "n03"},
		},
		{
			name:  "padding width comes from the low bound",
			input: "n[08-11]",
			want:  []string{"n08", "n09", "n10", "n11"},
		},
		{
			name:  "unpadded bound does not pad",
			input: "n[8-11]",
			want:  []string{"n8", "n9", "n10", "n11"},
		},
		{
			name:  "several bracket groups in one name",
			input: "r[1-2]n[01-02]",
			want:  []string{"r1n01", "r1n02", "r2n01", "r2n02"},
		},
		{
			name:  "text after the bracket",
			input: "n[1-2]x",
			want:  []string{"n1x", "n2x"},
		},
		{
			// The case the bracket-aware split exists for: a top-level comma
			// and a comma inside a set in the same expression.
			name:  "commas inside and outside brackets",
			input: "na1,nb[1-2,5],nc3",
			want:  []string{"na1", "nb1", "nb2", "nb5", "nc3"},
		},
		{
			name:  "single value in brackets",
			input: "n[7]",
			want:  []string{"n7"},
		},
		{
			name:  "range of one",
			input: "n[4-4]",
			want:  []string{"n4"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "whitespace only",
			input: "   ",
			want:  nil,
		},
		{
			name:  "surrounding whitespace is trimmed",
			input: " na1 , nb2 ",
			want:  []string{"na1", "nb2"},
		},
		{
			name:  "empty entries are skipped",
			input: "na1,,nb2",
			want:  []string{"na1", "nb2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, expandHostList(tt.input))
		})
	}
}

// A malformed expression must never make a node disappear: the collectors read
// this to decide which series to expose, and a dropped name is exactly the
// silent metric loss #254 is meant to remove. Verbatim is the safe failure.
func TestExpandHostListKeepsMalformedExpressionsVerbatim(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "unbalanced opening bracket",
			input: "n[1-3",
			want:  []string{"n[1-3"},
		},
		{
			name:  "non-numeric set",
			input: "n[a-c]",
			want:  []string{"n[a-c]"},
		},
		{
			name:  "descending range",
			input: "n[3-1]",
			want:  []string{"n[3-1]"},
		},
		{
			name:  "empty set",
			input: "n[]",
			want:  []string{"n[]"},
		},
		{
			name:  "one bad item invalidates only its own group",
			input: "na1,nb[1-x],nc3",
			want:  []string{"na1", "nb[1-x]", "nc3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, expandHostList(tt.input))
		})
	}
}

// An absurd range must cost a truncated result, not the exporter's memory.
func TestExpandHostListRefusesAnAbsurdRange(t *testing.T) {
	got := expandHostList(fmt.Sprintf("n[1-%d]", maxExpandedHosts+1))
	assert.Equal(t, []string{fmt.Sprintf("n[1-%d]", maxExpandedHosts+1)}, got)
}
