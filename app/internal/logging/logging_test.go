package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewWritesStructuredText(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output)
	logger.Info("process.start", "component", "runtime", "status", "started")

	got := output.String()
	for _, want := range []string{"msg=process.start", "component=runtime", "status=started"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output %q missing %q", got, want)
		}
	}
}

func TestNormalizeNilReturnsUsableLogger(t *testing.T) {
	logger := Normalize(nil)
	if logger == nil {
		t.Fatal("Normalize(nil) returned nil")
	}
	logger.Info("test")
}
