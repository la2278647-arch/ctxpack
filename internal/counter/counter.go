// Package counter estimates token counts for text using a tiktoken-style
// pre-tokenization regex plus a calibrated per-chunk heuristic.
//
// It deliberately avoids a heavyweight BPE encoder and any runtime data
// download, so the binary is fully self-contained and works offline. The
// estimate targets ±15% of real GPT-4o/o200k tokenization for mixed
// code+prose, which is accurate enough to answer "will this fit?" and to drive
// budget-aware packing. The counter is behind an interface so a precise
// tokenizer can be substituted later without touching callers.
package counter

import (
	"regexp"
	"strings"
	"sync"
)

// Model describes an LLM and its usable context window.
type Model struct {
	Name          string
	ContextWindow int    // total context tokens
	Vendor        string // openai | anthropic | google | meta | mistral | ...
}

// Estimate is the result of counting text.
type Estimate struct {
	Tokens int
	Chars  int
	Bytes  int
}

// Counter estimates tokens for arbitrary text.
type Counter interface {
	Estimate(text string) Estimate
}

// Default is the pre-tokenization + calibrated chunk estimator.
type Default struct {
	re *regexp.Regexp
}

var (
	defaultOnce sync.Once
	defaultInst *Default
)

// NewDefault returns a configured default estimator.
func NewDefault() *Default {
	defaultOnce.Do(func() {
		// RE2-compatible simplification of tiktoken's cl100k/o200k pattern.
		// Groupings: contractions, optional-space+letters, optional-space+1-3
		// digits, optional-space+symbols, runs of whitespace.
		pat := `'s|'t|'re|'ve|'m|'ll|'d| ?\p{L}+| ?\p{N}{1,3}| ?[^\s\p{L}\p{N}]+|\s+`
		defaultInst = &Default{re: regexp.MustCompile(pat)}
	})
	return defaultInst
}

// Estimate computes a token estimate for text.
func (d *Default) Estimate(text string) Estimate {
	if text == "" {
		return Estimate{}
	}
	chunks := d.re.FindAllString(text, -1)
	tokens := 0
	bytes := len(text)
	for _, c := range chunks {
		// Each pre-token becomes ~1 BPE token minimum; longer chunks add at
		// ~4 chars/token for prose and ~2.7 chars/token for symbol-heavy code.
		// A blended 3.5 divisor calibrated against mixed repos performs best.
		n := len(c)
		est := (n*10 + 34) / 35 // ≈ n/3.5, rounded, ≥1 for n>0
		if est < 1 {
			est = 1
		}
		tokens += est
	}
	// Floor at the naive bytes/4 so we never wildly under-report huge files.
	if floor := (bytes + 3) / 4; tokens < floor {
		tokens = floor
	}
	return Estimate{Tokens: tokens, Chars: len([]rune(text)), Bytes: bytes}
}

// EstimateBytes is a convenience for byte slices (handles invalid UTF-8 by
// counting runes where possible).
func (d *Default) EstimateBytes(b []byte) Estimate {
	return d.Estimate(string(b))
}

// EstimateSize estimates tokens from a byte count alone, for files whose
// content was not read (too large for MaxFileSize, or ReadContent is false).
// It applies the same bytes-per-token floor that Estimate uses, so a repo map
// built without reading content reports the same magnitude one built with it.
func (d *Default) EstimateSize(bytes int64) Estimate {
	n := int(bytes)
	return Estimate{Tokens: (n + 3) / 4, Chars: n, Bytes: n}
}

// ModelRegistry maps model identifiers to their metadata.
var models = []Model{
	{"gpt-3.5-turbo", 16385, "openai"},
	{"gpt-4", 8192, "openai"},
	{"gpt-4-turbo", 128000, "openai"},
	{"gpt-4o", 128000, "openai"},
	{"gpt-4o-mini", 128000, "openai"},
	{"o1", 200000, "openai"},
	{"o3", 200000, "openai"},
	{"claude-3-haiku", 200000, "anthropic"},
	{"claude-3-sonnet", 200000, "anthropic"},
	{"claude-3-opus", 200000, "anthropic"},
	{"claude-3.5-sonnet", 200000, "anthropic"},
	{"claude-3.5-haiku", 200000, "anthropic"},
	{"gemini-1.5-pro", 2000000, "google"},
	{"gemini-1.5-flash", 1000000, "google"},
	{"gemini-2.0-flash", 1048576, "google"},
	{"llama-3.1-405b", 128000, "meta"},
	{"mistral-large", 128000, "mistral"},
	{"deepseek-v3", 128000, "deepseek"},
	{"qwen2.5", 128000, "alibaba"},
}

// LookupModel finds a model by exact or prefix match (case-insensitive).
func LookupModel(name string) (Model, bool) {
	q := strings.ToLower(name)
	// Exact match first.
	for _, m := range models {
		if strings.ToLower(m.Name) == q {
			return m, true
		}
	}
	// Prefix match.
	for _, m := range models {
		if strings.HasPrefix(strings.ToLower(m.Name), q) || strings.HasPrefix(q, strings.ToLower(m.Name)) {
			return m, true
		}
	}
	return Model{}, false
}

// Models returns all known models.
func Models() []Model {
	out := make([]Model, len(models))
	copy(out, models)
	return out
}

// Fit describes how an estimate fits a model's context.
type Fit struct {
	Model   Model
	Used    int
	Limit   int
	Fits    bool
	PctUsed float64
}

// FitsModel reports whether est fits within model m, leaving a margin.
func FitsModel(est Estimate, m Model, reserve int) Fit {
	limit := m.ContextWindow - reserve
	if limit < 0 {
		limit = 0
	}
	pct := 0.0
	if limit > 0 {
		pct = float64(est.Tokens) / float64(limit) * 100
	}
	return Fit{
		Model:   m,
		Used:    est.Tokens,
		Limit:   limit,
		Fits:    est.Tokens <= limit,
		PctUsed: pct,
	}
}
