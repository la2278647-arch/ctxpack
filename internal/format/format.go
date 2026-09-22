// Package format renders a packed bundle of files into a single LLM-ready
// document. Supported formats: xml, markdown, json, text. The XML format
// wraps each file's content in CDATA so it survives any content unchanged,
// which makes it the most robust choice for feeding arbitrary source into a
// model.
package format

import (
	"encoding/json"
	"fmt"
	"strings"
)

// File is one file within a bundle, with pre-computed token/size stats.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Tokens  int    `json:"tokens"`
	Bytes   int    `json:"bytes"`
	Binary  bool   `json:"binary,omitempty"`
}

// Bundle is the full packed output.
type Bundle struct {
	Root        string `json:"root"`
	Files       []File `json:"files"`
	TotalTokens int    `json:"total_tokens"`
	TotalBytes  int    `json:"total_bytes"`
	Skipped     int    `json:"skipped"`
	// Omitted lists the paths a token budget left out, and OmittedTokens is
	// what they would have cost. Both stay empty unless a budget was applied,
	// so a caller always sees the cut instead of an unexplained shortfall.
	Omitted       []string `json:"omitted,omitempty"`
	OmittedTokens int      `json:"omitted_tokens,omitempty"`
}

// Format is the output style.
type Format string

const (
	XML      Format = "xml"
	Markdown Format = "markdown"
	JSON     Format = "json"
	Text     Format = "text"
)

// Render renders b in the requested format.
func Render(b *Bundle, f Format) string {
	switch f {
	case Markdown:
		return renderMarkdown(b)
	case JSON:
		return renderJSON(b)
	case Text:
		return renderText(b)
	default:
		return renderXML(b)
	}
}

func renderXML(b *Bundle) string {
	var sb strings.Builder
	sb.WriteString("<repository>\n")
	sb.WriteString("  <meta>\n")
	fmt.Fprintf(&sb, "    <root>%s</root>\n", xmlEscape(b.Root))
	fmt.Fprintf(&sb, "    <fileCount>%d</fileCount>\n", len(b.Files))
	fmt.Fprintf(&sb, "    <totalTokens>%d</totalTokens>\n", b.TotalTokens)
	fmt.Fprintf(&sb, "    <totalBytes>%d</totalBytes>\n", b.TotalBytes)
	fmt.Fprintf(&sb, "    <skipped>%d</skipped>\n", b.Skipped)
	sb.WriteString("  </meta>\n")
	sb.WriteString("  <files>\n")
	for _, f := range b.Files {
		// xmlEscape, not %q: Go's string quoting is not XML escaping, so a
		// path containing '&' or '<' emitted an entity that was not there.
		fmt.Fprintf(&sb, "    <file path=\"%s\" tokens=\"%s\" bytes=\"%s\">\n",
			xmlEscape(f.Path), itoa(f.Tokens), itoa(f.Bytes))
		if f.Binary {
			sb.WriteString("      <content binary=\"true\"/>\n")
		} else {
			// "]]>" inside the content would close the CDATA section early and
			// leave the rest of the file parsed as markup, so emit one CDATA
			// section per fragment.
			//
			// The element is hugged to the CDATA so the parsed text is the
			// file's bytes and nothing else: a newline before or after would
			// leak into the round trip, so every file read back gained a
			// leading and trailing blank line.
			sb.WriteString("      <content>")
			parts := strings.Split(f.Content, "]]>")
			for i, part := range parts {
				if i > 0 {
					// The separator itself cannot live inside CDATA, so write
					// it in the text context with the '>' escaped. Without
					// this the split content loses its "]]>" sequence.
					sb.WriteString("]]&gt;")
				}
				sb.WriteString("<![CDATA[")
				sb.WriteString(part)
				sb.WriteString("]]>")
			}
			sb.WriteString("</content>\n")
		}
		sb.WriteString("    </file>\n")
	}
	sb.WriteString("  </files>\n")
	writeOmittedXML(&sb, b)
	sb.WriteString("</repository>\n")
	return sb.String()
}

// writeOmittedXML reports the files the budget left out. Omitted entirely when
// nothing was cut, so an unlimited pack keeps its current shape.
func writeOmittedXML(sb *strings.Builder, b *Bundle) {
	if len(b.Omitted) == 0 {
		return
	}
	fmt.Fprintf(sb, "  <omitted count=\"%d\" tokens=\"%d\">\n", len(b.Omitted), b.OmittedTokens)
	for _, p := range b.Omitted {
		fmt.Fprintf(sb, "    <path>%s</path>\n", xmlEscape(p))
	}
	sb.WriteString("  </omitted>\n")
}

func renderMarkdown(b *Bundle) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Repository: %s\n\n", b.Root)
	fmt.Fprintf(&sb, "- Files: %d  | Tokens: ~%d  | Bytes: %s  | Skipped: %d\n\n",
		len(b.Files), b.TotalTokens, humanBytes(b.TotalBytes), b.Skipped)
	sb.WriteString("---\n\n")
	for _, f := range b.Files {
		fmt.Fprintf(&sb, "## `%s` (%d tokens, %s)\n\n", f.Path, f.Tokens, humanBytes(f.Bytes))
		if f.Binary {
			sb.WriteString("_(binary file — content omitted)_\n\n")
		} else {
			// The closing fence must start on its own line, but the file's
			// bytes must not gain a newline to get there: add one only when
			// the source lacks it.
			fmt.Fprintf(&sb, "```%s\n%s", langHint(f.Path), f.Content)
			if !strings.HasSuffix(f.Content, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("```\n\n")
		}
		sb.WriteString("---\n\n")
	}
	writeOmittedMarkdown(&sb, b)
	return sb.String()
}

// writeOmittedMarkdown reports the files the budget left out, so a truncated
// bundle says so instead of looking like the repository was small. Left empty
// when nothing was cut.
func writeOmittedMarkdown(sb *strings.Builder, b *Bundle) {
	if len(b.Omitted) == 0 {
		return
	}
	fmt.Fprintf(sb, "## Omitted by budget (%d files, ~%d tokens)\n\n",
		len(b.Omitted), b.OmittedTokens)
	for _, p := range b.Omitted {
		fmt.Fprintf(sb, "- `%s`\n", p)
	}
	sb.WriteString("\n")
}

func renderJSON(b *Bundle) string {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func renderText(b *Bundle) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Repository: %s\nFiles: %d | Tokens: ~%d | Bytes: %s\n\n",
		b.Root, len(b.Files), b.TotalTokens, humanBytes(b.TotalBytes))
	for _, f := range b.Files {
		fmt.Fprintf(&sb, "==== %s (%d tokens) ====\n", f.Path, f.Tokens)
		if f.Binary {
			sb.WriteString("[binary file — content omitted]\n")
		} else {
			sb.WriteString(f.Content)
			if !strings.HasSuffix(f.Content, "\n") {
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}
	writeOmittedText(&sb, b)
	return sb.String()
}

// writeOmittedText reports the files the budget left out. Left empty when
// nothing was cut.
func writeOmittedText(sb *strings.Builder, b *Bundle) {
	if len(b.Omitted) == 0 {
		return
	}
	fmt.Fprintf(sb, "==== omitted by budget (%d files, ~%d tokens) ====\n",
		len(b.Omitted), b.OmittedTokens)
	for _, p := range b.Omitted {
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func humanBytes(n int) string {
	const unit = 1024
	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB", "ZB"}
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	// i counts whole divisions by unit, so units[i] is the right label.
	// The bound stops a pathological size from running past the table.
	f, i := float64(n), 0
	for f >= unit && i < len(units)-1 {
		f /= unit
		i++
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

// langHint returns a markdown code-fence language hint from the file path.
func langHint(path string) string {
	low := strings.ToLower(path)
	switch {
	case strings.HasSuffix(low, ".go"):
		return "go"
	case strings.HasSuffix(low, ".ts") || strings.HasSuffix(low, ".tsx"):
		return "ts"
	case strings.HasSuffix(low, ".js") || strings.HasSuffix(low, ".jsx"):
		return "js"
	case strings.HasSuffix(low, ".py"):
		return "python"
	case strings.HasSuffix(low, ".rs"):
		return "rust"
	case strings.HasSuffix(low, ".java"):
		return "java"
	case strings.HasSuffix(low, ".kt"):
		return "kotlin"
	case strings.HasSuffix(low, ".rb"):
		return "ruby"
	case strings.HasSuffix(low, ".sh") || strings.HasSuffix(low, ".bash"):
		return "bash"
	case strings.HasSuffix(low, ".yml") || strings.HasSuffix(low, ".yaml"):
		return "yaml"
	case strings.HasSuffix(low, ".json"):
		return "json"
	case strings.HasSuffix(low, ".md"):
		return "markdown"
	case strings.HasSuffix(low, ".sql"):
		return "sql"
	case strings.HasSuffix(low, ".c") || strings.HasSuffix(low, ".h"):
		return "c"
	case strings.HasSuffix(low, ".cpp") || strings.HasSuffix(low, ".hpp") || strings.HasSuffix(low, ".cc"):
		return "cpp"
	case strings.HasSuffix(low, ".cs"):
		return "csharp"
	case strings.HasSuffix(low, ".html"), strings.HasSuffix(low, ".htm"):
		return "html"
	case strings.HasSuffix(low, ".css"):
		return "css"
	case strings.HasSuffix(low, ".dockerfile") || strings.HasSuffix(low, "dockerfile"):
		return "dockerfile"
	}
	return ""
}
