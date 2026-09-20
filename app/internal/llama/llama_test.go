package llama

import (
	"context"
	"errors"
	"os"
	"strings"
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

func TestNativeGenerateClearsContextMemoryBeforeTokenization(t *testing.T) {
	source, err := os.ReadFile("native.cpp")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	clearCall := "llama_memory_clear(llama_get_memory(engine->context), true);"
	clearAt := strings.Index(text, clearCall)
	tokenizeAt := strings.Index(text, "llama_tokenize(")
	if clearAt < 0 {
		t.Fatalf("native generation does not clear context memory with pinned API")
	}
	if tokenizeAt < 0 || clearAt > tokenizeAt {
		t.Fatalf("context memory clear must precede tokenization: clear=%d tokenize=%d", clearAt, tokenizeAt)
	}
}
