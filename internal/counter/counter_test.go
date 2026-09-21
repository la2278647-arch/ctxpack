package counter

import (
	"strings"
	"testing"
)

func TestEstimateEmpty(t *testing.T) {
	c := NewDefault()
	if e := c.Estimate(""); e.Tokens != 0 {
		t.Fatalf("empty text tokens = %d, want 0", e.Tokens)
	}
}

func TestEstimateScales(t *testing.T) {
	c := NewDefault()
	small := c.Estimate("hello world")
	big := c.Estimate(strings.Repeat("hello world ", 1000))
	if big.Tokens <= small.Tokens {
		t.Fatalf("big (%d) should exceed small (%d)", big.Tokens, small.Tokens)
	}
	if small.Tokens < 2 {
		t.Fatalf("small token estimate %d implausibly low", small.Tokens)
	}
}

func TestEstimateReasonableRange(t *testing.T) {
	c := NewDefault()
	// ~1000 English words ≈ ~1300 tokens for GPT-4; estimator should land
	// within a sane band (not 10, not 100000).
	text := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 100)
	e := c.Estimate(text)
	if e.Tokens < 500 || e.Tokens > 3000 {
		t.Fatalf("token estimate %d outside plausible band for %d chars", e.Tokens, e.Bytes)
	}
}

func TestLookupModel(t *testing.T) {
	if _, ok := LookupModel("gpt-4o"); !ok {
		t.Fatal("expected gpt-4o to be known")
	}
	if _, ok := LookupModel("GPT-4O"); !ok { // case-insensitive
		t.Fatal("expected case-insensitive lookup to work")
	}
	if _, ok := LookupModel("does-not-exist"); ok {
		t.Fatal("unknown model should not match")
	}
}

func TestFitsModel(t *testing.T) {
	m := Model{Name: "gpt-4o", ContextWindow: 128000}
	f := FitsModel(Estimate{Tokens: 50000}, m, 4096)
	if !f.Fits {
		t.Fatal("50000 tokens should fit gpt-4o")
	}
	f2 := FitsModel(Estimate{Tokens: 200000}, m, 4096)
	if f2.Fits {
		t.Fatal("200000 tokens should not fit gpt-4o")
	}
}
