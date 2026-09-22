// Package counter estimates token counts for text using a tiktoken-style
// pre-tokenization regex plus a calibrated per-chunk heuristic.
//
// It deliberately avoids a heavyweight BPE encoder and any runtime data
// download, so the binary is fully self-contained and works offline. The
// estimate over-reports real GPT-4o/o200k tokenization by a factor that
// depends on the text — around 2x for chunk-dense code, 1.5x for prose — so
// it is a sizing number, not a billing one. That is accurate enough to answer
// "will this fit?" and to drive budget-aware packing, and the counter is
// behind an interface so a precise tokenizer can be substituted later without
// touching callers.
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
	for _, c := range chunks {
		tokens += chunkTokens(len(c))
	}
	bytes := len(text)
	return Estimate{Tokens: floorTokens(tokens, bytes), Chars: len([]rune(text)), Bytes: bytes}
}

// chunkTokens estimates the BPE tokens in one regex chunk of n bytes.
//
// Each pre-token becomes ~1 BPE token minimum; longer chunks add at ~4
// chars/token for prose and ~2.7 chars/token for symbol-heavy code. A blended
// 3.5 divisor calibrated against mixed repos performs best.
//
// The clamp is insurance rather than a live path: no alternative in the pattern
// matches the empty string, so n is always >= 1 and the division above is
// already >= 1. It is kept so a future pattern that does match empty strings
// reports one token for the chunk instead of silently under-reporting it as
// zero. See TestEstimateNeverYieldsEmptyChunks.
func chunkTokens(n int) int {
	est := (n*10 + 34) / 35 // ≈ n/3.5, rounded
	if est < 1 {
		est = 1
	}
	return est
}

// floorTokens never under-reports a file by more than the naive bytes/4 guess,
// so an estimate can not regress into absurd territory if the chunk formula
// above is ever changed. Today it is insurance that does not bind: the chunks
// account for all bytes and sum to at least bytes/3.5. See
// TestEstimateFloorDoesNotBind.
func floorTokens(tokens, bytes int) int {
	if floor := (bytes + 3) / 4; tokens < floor {
		return floor
	}
	return tokens
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
	if n < 0 {
		// A negative size is caller error, not a signal. Without the clamp the
		// (n+3)/4 division would round toward zero and hand back a negative
		// token count, which would make a bundle's totals go backwards.
		n = 0
	}
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

// LookupModel finds a model by exact or prefix match (case-insensitive). An
// empty query matches nothing: with an empty q, HasPrefix(m.Name, "") is true
// for every registered model, so the lookup used to return the first one
// whenever a caller passed an unset value.
func LookupModel(name string) (Model, bool) {
	if name == "" {
		return Model{}, false
	}
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

// ReplyReserve is the token budget held back for a model's reply. A bundle
// that exactly fills a context window leaves no room to answer, so every fit
// calculation subtracts this from the window. It lives here, next to FitsModel
// which consumes it, rather than as a literal in each call site: the CLI, the
// JSON output and the MCP server must all read the same value.
const ReplyReserve = 4096

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
