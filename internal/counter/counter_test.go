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
	// New models (R35).
	if _, ok := LookupModel("gpt-4.1"); !ok {
		t.Fatal("expected gpt-4.1 to be known")
	}
	if _, ok := LookupModel("claude-4-sonnet"); !ok {
		t.Fatal("expected claude-4-sonnet to be known")
	}
	if _, ok := LookupModel("gemini-2.5-pro"); !ok {
		t.Fatal("expected gemini-2.5-pro to be known")
	}
	if _, ok := LookupModel("deepseek-r1"); !ok {
		t.Fatal("expected deepseek-r1 to be known")
	}
}

func TestLookupModelPrefix(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"claude", "claude-3-haiku"},                    // bare family prefix
		{"GEMINI-1.5", "gemini-1.5-pro"},                // case-insensitive family
		{"o1", "o1"},                                    // exact wins over prefix
		{"claude-3.5-sonnet-2024", "claude-3.5-sonnet"}, // query longer than a known name
		{"deepseek", "deepseek-v3"},
		{"llama", "llama-3.1-405b"},
		{"gpt-4.1", "gpt-4.1"},           // new model
		{"claude-4", "claude-4-sonnet"},  // new model prefix
		{"gemini-2.5", "gemini-2.5-pro"}, // new model prefix
		{"deepseek-r", "deepseek-r1"},    // new model prefix
	}
	for _, tc := range cases {
		got, ok := LookupModel(tc.query)
		if !ok {
			t.Errorf("LookupModel(%q) not found", tc.query)
			continue
		}
		if got.Name != tc.want {
			t.Errorf("LookupModel(%q) = %q, want %q", tc.query, got.Name, tc.want)
		}
	}
}

func TestLookupModelEmptyAndWhitespace(t *testing.T) {
	// An empty query matched every registered model through
	// HasPrefix(name, "") and returned the first one.
	if got, ok := LookupModel(""); ok {
		t.Errorf("LookupModel(\"\") = %+v, ok; want no match", got)
	}
	// Whitespace is not empty but is not a model either.
	if _, ok := LookupModel("   "); ok {
		t.Error("LookupModel(\"   \") must not match")
	}
}

func TestModelsIsCopy(t *testing.T) {
	all := Models()
	if len(all) != 23 {
		t.Fatalf("Models() returned %d entries, want 23", len(all))
	}
	seen := map[string]bool{}
	for _, m := range all {
		if m.Name == "" || m.ContextWindow <= 0 || m.Vendor == "" {
			t.Errorf("model %+v has a zero field", m)
		}
		if m.ContextWindow < 1000 {
			t.Errorf("model %q has an implausibly small window: %d", m.Name, m.ContextWindow)
		}
		seen[m.Name] = true
	}
	// Two families share a window size; names must not collide.
	if len(seen) != len(all) {
		t.Error("Models() returned duplicate names")
	}
	// The returned slice must not alias the registry.
	all[0].Name = "mutated"
	if got, ok := LookupModel("mutated"); ok || got.Name == "mutated" {
		t.Error("Models() returned a slice that aliases the registry")
	}
}

func TestFitsModelPercentAndReserve(t *testing.T) {
	m := Model{Name: "gpt-4o", ContextWindow: 128000}
	f := FitsModel(Estimate{Tokens: 50000}, m, 4096)
	if !f.Fits {
		t.Fatal("50000 tokens should fit gpt-4o")
	}
	if f.Limit != 123904 {
		t.Errorf("Limit = %d, want 123904", f.Limit)
	}
	if f.PctUsed < 39 || f.PctUsed > 41 {
		t.Errorf("PctUsed = %v, want about 40.4", f.PctUsed)
	}
	f2 := FitsModel(Estimate{Tokens: 200000}, m, 4096)
	if f2.Fits {
		t.Fatal("200000 tokens should not fit gpt-4o")
	}
	if f2.PctUsed < 150 || f2.PctUsed > 170 {
		t.Errorf("PctUsed = %v for an over-limit estimate", f2.PctUsed)
	}
}

func TestFitsModelReserveExceedsWindow(t *testing.T) {
	// A reserve larger than the window must clamp the limit to zero, not go
	// negative.
	f := FitsModel(Estimate{Tokens: 100}, Model{Name: "tiny", ContextWindow: 100}, 500)
	if f.Limit != 0 {
		t.Errorf("Limit = %d, want 0", f.Limit)
	}
	if f.Fits {
		t.Error("nothing fits a model whose window is fully reserved")
	}
	if f.PctUsed != 0 {
		t.Errorf("PctUsed = %v, want 0 when the limit is zero", f.PctUsed)
	}
	// Exactly zero tokens does fit.
	if !FitsModel(Estimate{Tokens: 0}, Model{Name: "tiny", ContextWindow: 100}, 500).Fits {
		t.Error("an empty estimate must fit")
	}
	// An empty estimate against a real window reports zero usage.
	if f := FitsModel(Estimate{Tokens: 0}, Model{Name: "gpt-4o", ContextWindow: 128000}, 4096); !f.Fits || f.PctUsed != 0 {
		t.Errorf("empty estimate: %+v", f)
	}
	// The boundary is inclusive: exactly at the limit still fits.
	atLimit := FitsModel(Estimate{Tokens: 123904}, Model{Name: "gpt-4o", ContextWindow: 128000}, 4096)
	if !atLimit.Fits {
		t.Error("an estimate exactly at the limit must fit")
	}
	over := FitsModel(Estimate{Tokens: 123905}, Model{Name: "gpt-4o", ContextWindow: 128000}, 4096)
	if over.Fits {
		t.Error("an estimate one token over the limit must not fit")
	}
}

func TestEstimateChunkDecomposition(t *testing.T) {
	c := NewDefault()
	cases := []struct {
		text string
		want []string
	}{
		{"don't", []string{"don", "'t"}},
		{"I'm here", []string{"I", "'m", " here"}},
		{"we're", []string{"we", "'re"}},
		// Digits are capped at 1-3 per chunk, so a long number splits.
		{"20240101", []string{"202", "401", "01"}},
		{"42", []string{"42"}},
		{"123456", []string{"123", "456"}},
		// A leading space is absorbed into a letter or digit chunk, but a
		// leading space before punctuation becomes its own whitespace chunk.
		{"hello", []string{"hello"}},
		{" hello", []string{" hello"}},
		{"hello!", []string{"hello", "!"}},
		{" a", []string{" a"}},
		{" !", []string{" !"}},
		{"a b c", []string{"a", " b", " c"}},
		// Consecutive whitespace collapses into one chunk.
		{"a\n\n\nb", []string{"a", "\n\n\n", "b"}},
		// Punctuation runs are one chunk, however long.
		{"!!!---", []string{"!!!---"}},
		// Mixed symbol runs split on letters and digits.
		{"a+b=c", []string{"a", "+", "b", "=", "c"}},
		// Non-ASCII letters are letters.
		{"café résumé", []string{"café", " résumé"}},
		// Emoji are symbols, not letters.
		{"hi 😀", []string{"hi", " 😀"}},
	}
	for _, tc := range cases {
		got := c.re.FindAllString(tc.text, -1)
		if len(got) != len(tc.want) {
			t.Errorf("chunks(%q) = %v, want %v", tc.text, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("chunks(%q)[%d] = %q, want %q", tc.text, i, got[i], tc.want[i])
			}
		}
	}
}

func TestEstimateNeverYieldsEmptyChunks(t *testing.T) {
	// Every alternative in the pattern requires at least one character, so no
	// chunk is ever empty and the `est < 1` clamp is unreachable. This battery
	// covers the pathological edges: NULs, control characters, surrogates,
	// non-breaking spaces, and lone punctuation.
	c := NewDefault()
	for _, text := range []string{
		"a",
		"\x00",
		"\x00\x00\x00",
		"\x7f\x01\x02",
		" ", // non-breaking space
		" ", // thin space
		"🎯🎯🎯",
		"'",
		"`\"'\"`",
		"\t\r\n\f ",
		"é",
		"😀",
		strings.Repeat("a", 100000),
		strings.Repeat("!", 100000),
		strings.Repeat("123", 10000),
	} {
		chunks := c.re.FindAllString(text, -1)
		for _, ch := range chunks {
			if ch == "" {
				t.Fatalf("empty chunk from %q", short(text))
			}
		}
		// The chunks must also account for every byte of the input.
		total := 0
		for _, ch := range chunks {
			total += len(ch)
		}
		if total != len(text) {
			t.Errorf("chunks of %q cover %d bytes, want %d", short(text), total, len(text))
		}
	}
}

// short is a fixed-width preview for error messages.
func short(s string) string {
	if len(s) > 40 {
		return s[:40]
	}
	return s
}

func TestEstimateFloorDoesNotBind(t *testing.T) {
	// (n*10+34)/35 is at least n/3.5 for every n >= 1, and the chunks account
	// for all bytes, so the per-chunk sum is always >= bytes/3.5, well above
	// the bytes/4 floor. Asserting strictly above the floor proves the floor
	// never binds, so a future formula change that would make it bind fails
	// loudly here.
	c := NewDefault()
	for _, text := range []string{
		"the quick brown fox jumps over the lazy dog.",
		strings.Repeat("abcdef ", 100000),
		strings.Repeat("1234567890 ", 100000),
		strings.Repeat("!!!!! ", 100000),
		strings.Repeat("café résumé ", 50000),
		strings.Repeat("\x00\x00\x00 ", 50000),
	} {
		e := c.Estimate(text)
		floor := (e.Bytes + 3) / 4
		if e.Tokens <= floor {
			t.Errorf("tokens (%d) not above the bytes/4 floor (%d): the floor would bind",
				e.Tokens, floor)
		}
		// The ratio must be at least bytes/3.5, which is what the 3.5 divisor
		// promises. 7 tokens per 25 bytes is exactly n/3.57.
		if e.Tokens*25 < e.Bytes*7 {
			t.Errorf("tokens (%d) below bytes/3.5 for %d bytes", e.Tokens, e.Bytes)
		}
	}
}

// chunkTokens and floorTokens are the two insurance clamps in Estimate, pulled
// out so each can be pinned directly rather than only observed through the
// regex.

func TestChunkTokens(t *testing.T) {
	// The formula is ~n/3.5 rounded, never below 1.
	for _, tc := range []struct {
		in   int
		want int
	}{
		{0, 1}, {-1, 1}, {1, 1}, {2, 1}, {3, 1}, {4, 2}, {5, 2},
		{10, 3}, {34, 10}, {35, 10}, {100, 29}, {1000, 286},
	} {
		if got := chunkTokens(tc.in); got != tc.want {
			t.Errorf("chunkTokens(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestFloorTokens(t *testing.T) {
	if got := floorTokens(0, 100); got != 25 {
		t.Errorf("floorTokens(0,100) = %d, want 25", got)
	}
	if got := floorTokens(25, 100); got != 25 {
		t.Errorf("floorTokens(25,100) = %d, want 25 (at the floor, unchanged)", got)
	}
	if got := floorTokens(40, 100); got != 40 {
		t.Errorf("floorTokens(40,100) = %d, want 40 (above the floor, kept)", got)
	}
	if got := floorTokens(0, 0); got != 0 {
		t.Errorf("floorTokens(0,0) = %d, want 0", got)
	}
}

// The existing TestEstimateFloorDoesNotBind only samples large texts. A single
// chunk is the worst case for the floor: it earns the fewest of the +34 byte
// bonuses in the chunk formula, so if the floor can bind anywhere it binds on
// a one-chunk text. Sweep every length to prove it cannot.
func TestEstimateFloorNeverBindsOneChunk(t *testing.T) {
	c := NewDefault()
	for n := 1; n <= 500; n++ {
		e := c.Estimate(strings.Repeat("a", n))
		if e.Tokens*4 <= e.Bytes {
			t.Errorf("n=%d: tokens (%d) not strictly above the bytes/4 floor: the floor binds",
				n, e.Tokens)
		}
	}
}

func TestEstimateBytesAndRuneCount(t *testing.T) {
	c := NewDefault()
	text := "café ✓"
	e1 := c.Estimate(text)
	e2 := c.EstimateBytes([]byte(text))
	if e1 != e2 {
		t.Errorf("EstimateBytes(%q) = %+v, want Estimate = %+v", text, e2, e1)
	}
	if e2.Bytes != len(text) {
		t.Errorf("Bytes = %d, want %d", e2.Bytes, len(text))
	}
	if e2.Chars != 6 { // 6 runes: c a f é SPACE ✓
		t.Errorf("Chars = %d, want 6", e2.Chars)
	}
	if e2.Chars >= e2.Bytes {
		t.Errorf("Chars (%d) must be <= Bytes (%d)", e2.Chars, e2.Bytes)
	}
}

func TestEstimateBytesInvalidUTF8(t *testing.T) {
	c := NewDefault()
	// Invalid sequences must not panic and must not be counted as fewer bytes
	// than they occupy.
	e := c.EstimateBytes([]byte{0xff, 0xfe, 0xfd, 'a', 'b', 'c'})
	if e.Bytes != 6 {
		t.Errorf("Bytes = %d, want 6", e.Bytes)
	}
	if e.Tokens < 1 {
		t.Errorf("Tokens = %d, want >= 1", e.Tokens)
	}
}

func TestEstimateSize(t *testing.T) {
	c := NewDefault()
	cases := []struct {
		bytes  int64
		tokens int
	}{
		{0, 0},
		{1, 1},
		{3, 1},
		{4, 1},
		{5, 2},
		{7, 2},
		{8, 2},
		{40000, 10000},
		{1000000, 250000},
	}
	for _, tc := range cases {
		e := c.EstimateSize(tc.bytes)
		if e.Tokens != tc.tokens {
			t.Errorf("EstimateSize(%d).Tokens = %d, want %d", tc.bytes, e.Tokens, tc.tokens)
		}
		if e.Bytes != int(tc.bytes) || e.Chars != int(tc.bytes) {
			t.Errorf("EstimateSize(%d) = %+v, want Bytes and Chars = %d", tc.bytes, e, tc.bytes)
		}
	}
}

func TestEstimateSizeNegative(t *testing.T) {
	// A negative size is caller error, and the (n+3)/4 division rounds toward
	// zero, so an unclamped negative value produced a negative token count.
	c := NewDefault()
	for _, n := range []int64{-1, -3, -4, -5, -8, -100000} {
		e := c.EstimateSize(n)
		if e.Tokens < 0 {
			t.Errorf("EstimateSize(%d).Tokens = %d, want >= 0", n, e.Tokens)
		}
		if e.Bytes != 0 || e.Chars != 0 {
			t.Errorf("EstimateSize(%d) = %+v, want zeros", n, e)
		}
	}
}

func TestEstimateSizeMirrorsEstimate(t *testing.T) {
	// A file read from disk and one estimated only from its size must report
	// the same magnitude, otherwise the repo map and the pack disagree.
	//
	// They do not agree closely. Estimate runs at about one token per
	// pre-token, so chunk-dense code lands at 2x-3x the bytes/4 that
	// EstimateSize uses: for "func Foo() int {\n\treturn 0\n}\n" the estimator
	// returns 11 tokens where the size heuristic returns 7, and for ASCII prose
	// it is about 33% higher. A repo map built with ReadContent:false therefore
	// under-reports the same files the pack reports with content. The band
	// below is wide enough to hold that skew but narrow enough to fail if the
	// two diverge by more than an order of magnitude.
	c := NewDefault()
	cases := []string{
		strings.Repeat("hello world ", 10000),
		strings.Repeat("package main\nfunc main() {}\n", 20000),
		strings.Repeat("func Foo() int {\n\treturn 0\n}\n", 20000),
		strings.Repeat("!!! --- *** === ", 10000),
		strings.Repeat("café résumé ", 20000),
	}
	for _, text := range cases {
		read := c.Estimate(text)
		sizeOnly := c.EstimateSize(int64(len(text)))
		if sizeOnly.Tokens == 0 {
			t.Fatalf("EstimateSize returned zero for %d bytes", len(text))
		}
		got := read.Tokens * 100 / sizeOnly.Tokens
		if got < 100 || got > 350 {
			t.Errorf("Estimate(%q) = %d tokens vs EstimateSize = %d (ratio %d%%): "+
				"the two must agree within one order of magnitude",
				text[:24], read.Tokens, sizeOnly.Tokens, got)
		}
	}
}
