package format

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

func ZNodeContent(data []byte) string {
	if len(data) == 0 {
		return "<empty>"
	}

	if decoded, ok := tryGunzip(data); ok {
		data = decoded
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && json.Valid(trimmed) {
		var out bytes.Buffer
		if err := json.Indent(&out, trimmed, "", "  "); err == nil {
			return highlightJSON(out.String())
		}
	}

	if utf8.Valid(data) {
		return strings.TrimRight(string(data), "\n")
	}

	return strings.TrimRight(hex.Dump(data), "\n")
}

func DataSizeSummary(data []byte) string {
	compressed := len(data)
	if decoded, ok := tryGunzip(data); ok {
		return fmt.Sprintf("Size: %d bytes (compressed), %d bytes (uncompressed)", compressed, len(decoded))
	}
	return fmt.Sprintf("Size: %d bytes", compressed)
}

func tryGunzip(data []byte) ([]byte, bool) {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return nil, false
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	defer r.Close()

	decoded, err := io.ReadAll(r)
	if err != nil {
		return nil, false
	}
	return decoded, true
}

const (
	ansiReset   = "\x1b[0m"
	ansiBlue    = "\x1b[34m"
	ansiGreen   = "\x1b[32m"
	ansiCyan    = "\x1b[36m"
	ansiMagenta = "\x1b[35m"
)

func highlightJSON(pretty string) string {
	var b strings.Builder
	for i := 0; i < len(pretty); {
		ch := pretty[i]
		if ch == '"' {
			start := i
			i++
			for i < len(pretty) {
				if pretty[i] == '\\' {
					i += 2
					continue
				}
				if pretty[i] == '"' {
					i++
					break
				}
				i++
			}
			token := pretty[start:i]
			if isObjectKey(pretty, i) {
				b.WriteString(ansiBlue)
				b.WriteString(token)
				b.WriteString(ansiReset)
			} else {
				b.WriteString(ansiGreen)
				b.WriteString(token)
				b.WriteString(ansiReset)
			}
			continue
		}

		if lit, ok := readLiteral(pretty, i, "true"); ok {
			b.WriteString(ansiMagenta + lit + ansiReset)
			i += len(lit)
			continue
		}
		if lit, ok := readLiteral(pretty, i, "false"); ok {
			b.WriteString(ansiMagenta + lit + ansiReset)
			i += len(lit)
			continue
		}
		if lit, ok := readLiteral(pretty, i, "null"); ok {
			b.WriteString(ansiMagenta + lit + ansiReset)
			i += len(lit)
			continue
		}
		if num, ok := readNumber(pretty, i); ok {
			b.WriteString(ansiCyan + num + ansiReset)
			i += len(num)
			continue
		}

		b.WriteByte(ch)
		i++
	}
	return b.String()
}

func isObjectKey(s string, idx int) bool {
	for idx < len(s) && (s[idx] == ' ' || s[idx] == '\n' || s[idx] == '\r' || s[idx] == '\t') {
		idx++
	}
	return idx < len(s) && s[idx] == ':'
}

func readLiteral(s string, idx int, lit string) (string, bool) {
	if !strings.HasPrefix(s[idx:], lit) {
		return "", false
	}
	end := idx + len(lit)
	if idx > 0 && isIdentChar(s[idx-1]) {
		return "", false
	}
	if end < len(s) && isIdentChar(s[end]) {
		return "", false
	}
	return lit, true
}

func isIdentChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func readNumber(s string, idx int) (string, bool) {
	j := idx
	if s[j] == '-' {
		j++
		if j >= len(s) {
			return "", false
		}
	}
	startDigits := j
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == startDigits {
		return "", false
	}
	if j < len(s) && s[j] == '.' {
		j++
		fracStart := j
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == fracStart {
			return "", false
		}
	}
	if j < len(s) && (s[j] == 'e' || s[j] == 'E') {
		j++
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		expStart := j
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == expStart {
			return "", false
		}
	}
	token := s[idx:j]
	if _, err := strconv.ParseFloat(token, 64); err != nil {
		return "", false
	}
	return token, true
}
