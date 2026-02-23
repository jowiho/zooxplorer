package format

import (
	"bytes"
	"compress/gzip"
	"regexp"
	"strconv"
	"testing"
)

func TestZNodeContentPrettyJSON(t *testing.T) {
	in := []byte(`{"a":1,"b":{"c":2}}`)
	got := ZNodeContent(in)
	want := "{\n  \"a\": 1,\n  \"b\": {\n    \"c\": 2\n  }\n}"
	if stripANSI(got) != want {
		t.Fatalf("unexpected pretty JSON:\n%s", got)
	}
}

func TestZNodeContentPlainText(t *testing.T) {
	got := ZNodeContent([]byte("hello"))
	if got != "hello" {
		t.Fatalf("unexpected text: %q", got)
	}
}

func TestZNodeContentEmpty(t *testing.T) {
	got := ZNodeContent(nil)
	if got != "<empty>" {
		t.Fatalf("unexpected empty marker: %q", got)
	}
}

func TestZNodeContentGunzipText(t *testing.T) {
	got := ZNodeContent(gzipBytes(t, []byte("hello gzip")))
	if got != "hello gzip" {
		t.Fatalf("unexpected text: %q", got)
	}
}

func TestZNodeContentGunzipJSONPrettyPrint(t *testing.T) {
	got := ZNodeContent(gzipBytes(t, []byte(`{"a":1}`)))
	want := "{\n  \"a\": 1\n}"
	if stripANSI(got) != want {
		t.Fatalf("unexpected pretty JSON:\n%s", got)
	}
}

func TestDataSizeSummaryPlain(t *testing.T) {
	got := DataSizeSummary([]byte("hello"))
	if got != "Size: 5 bytes" {
		t.Fatalf("unexpected size summary: %q", got)
	}
}

func TestDataSizeSummaryCompressed(t *testing.T) {
	gz := gzipBytes(t, []byte("hello gzip"))
	got := DataSizeSummary(gz)
	want := "Size: " + strconv.Itoa(len(gz)) + " bytes (compressed), 10 bytes (uncompressed)"
	if got != want {
		t.Fatalf("unexpected size summary: %q", got)
	}
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return b.Bytes()
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}
