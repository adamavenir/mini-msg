package chat

import (
	"bytes"
	"os"
	"strings"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/formatters"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/styles"
)

const chromaStyleName = "dracula"

func highlightCodeBlocks(body string) string {
	if body == "" || os.Getenv("NO_COLOR") != "" {
		return body
	}

	lines := strings.Split(body, "\n")
	var out strings.Builder

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		fence, lang, ok := parseFence(line)
		if !ok {
			out.WriteString(line)
			if i < len(lines)-1 {
				out.WriteByte('\n')
			}
			continue
		}

		end := findClosingFence(lines, i+1, fence)
		if end == -1 {
			out.WriteString(line)
			if i < len(lines)-1 {
				out.WriteByte('\n')
			}
			continue
		}

		out.WriteString(line)
		out.WriteByte('\n')

		code := strings.Join(lines[i+1:end], "\n")
		out.WriteString(highlightCode(code, lang))
		out.WriteByte('\n')
		out.WriteString(lines[end])

		if end < len(lines)-1 {
			out.WriteByte('\n')
		}
		i = end
	}

	return out.String()
}

func parseFence(line string) (string, string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 3 {
		return "", "", false
	}

	fenceChar := trimmed[0]
	if fenceChar != '`' && fenceChar != '~' {
		return "", "", false
	}

	count := 0
	for count < len(trimmed) && trimmed[count] == fenceChar {
		count++
	}
	if count < 3 {
		return "", "", false
	}

	fence := trimmed[:count]
	rest := strings.TrimSpace(trimmed[count:])
	lang := ""
	if rest != "" {
		parts := strings.Fields(rest)
		if len(parts) > 0 {
			lang = parts[0]
		}
	}

	return fence, lang, true
}

func findClosingFence(lines []string, start int, fence string) int {
	for i := start; i < len(lines); i++ {
		if isClosingFence(lines[i], fence) {
			return i
		}
	}
	return -1
}

func isClosingFence(line string, fence string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < len(fence) {
		return false
	}

	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != fence[0] {
			return false
		}
	}
	return true
}

func highlightCode(code, lang string) string {
	if code == "" {
		return ""
	}

	lexer := resolveLexer(code, lang)
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}

	style := styles.Get(chromaStyleName)
	if style == nil {
		style = styles.Fallback
	}

	var buf bytes.Buffer
	if err := formatters.TTY256.Format(&buf, style, iterator); err != nil {
		return code
	}

	return strings.TrimSuffix(buf.String(), "\n")
}

func resolveLexer(code, lang string) chroma.Lexer {
	lang = strings.ToLower(strings.TrimSpace(lang))
	var lexer chroma.Lexer
	if lang != "" {
		lexer = lexers.Get(lang)
	}
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return chroma.Coalesce(lexer)
}
