package format

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
			return out.String()
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
