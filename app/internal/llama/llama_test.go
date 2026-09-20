package llama

import (
	"context"
	"errors"
	"testing"
)

func TestStubOpenIsExplicitlyUnavailable(t *testing.T) {
	_, err := Open(context.Background(), "functiongemma.gguf", 2)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestRequestRejectsInvalidLimits(t *testing.T) {
	if err := validateRequest(Request{Prompt: "x", Threads: 0, MaxTokens: 1}); err == nil {
		t.Fatal("accepted zero threads")
	}
	if err := validateRequest(Request{Prompt: "x", Threads: 1, MaxTokens: 0}); err == nil {
		t.Fatal("accepted zero max tokens")
	}
}
