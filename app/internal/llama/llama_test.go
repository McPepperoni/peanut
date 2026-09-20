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
	if err := validateRequest(Request{Prompt: "x", Threads: 1, MaxTokens: maxTokens + 1}); err == nil {
		t.Fatal("accepted max tokens above request limit")
	}
}

func TestNativeGenerateBoundsPromptBeforeCString(t *testing.T) {
	source, err := os.ReadFile("native_cgo.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	guardAt := strings.Index(text, "len(request.Prompt) > maxPromptBytes")
	cstringAt := strings.Index(text, "C.CString(request.Prompt)")
	if guardAt < 0 || cstringAt < 0 || guardAt > cstringAt {
		t.Fatalf("native Go path must reject oversized prompts before C.CString: guard=%d cstring=%d", guardAt, cstringAt)
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

func TestNativeBatchSizeCoversContextWindow(t *testing.T) {
	source, err := os.ReadFile("native.cpp")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "#define PEANUT_LLAMA_BATCH_SIZE 4096") {
		t.Fatal("native batch size must cover the maximum 4096-token context")
	}
}

func TestNativeContextBoundaryPrecedesPromptDecode(t *testing.T) {
	source, err := os.ReadFile("native.cpp")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	boundaryAt := strings.Index(text, "prompt_token_count > PEANUT_LLAMA_CONTEXT_SIZE - max_tokens")
	decodeAt := strings.Index(text, "llama_batch_get_one(prompt_tokens, (int32_t) prompt_token_count)")
	if boundaryAt < 0 || decodeAt < 0 || boundaryAt > decodeAt {
		t.Fatal("native context boundary must be checked before prompt decode")
	}
}

func TestNativePromptBoundsPrecedeTokenBufferAllocation(t *testing.T) {
	source, err := os.ReadFile("native.cpp")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	rawGuardAt := strings.Index(text, "prompt_bytes == PEANUT_LLAMA_MAX_PROMPT_BYTES")
	templateAt := strings.Index(text, "peanut_apply_chat_template(engine->model")
	tokenBudgetAt := strings.Index(text, "prompt_token_count > PEANUT_LLAMA_CONTEXT_SIZE - max_tokens")
	tokenBufferAt := strings.Index(text, "malloc(prompt_token_count * sizeof(*prompt_tokens))")
	if rawGuardAt < 0 || templateAt < 0 || rawGuardAt > templateAt {
		t.Fatal("native generation must bound raw prompt bytes before applying the chat template")
	}
	if tokenBudgetAt < 0 || tokenBufferAt < 0 || tokenBudgetAt > tokenBufferAt {
		t.Fatal("native generation must reject over-context prompts before allocating token storage")
	}
	if !strings.Contains(text, "#define PEANUT_LLAMA_MAX_PROMPT_BYTES") || !strings.Contains(text, "PEANUT_LLAMA_PROMPT_TOO_LARGE") {
		t.Fatal("native generation must expose an explicit oversized-prompt status")
	}
}

func TestNativeGenerateAppliesModelChatTemplateBeforeTokenization(t *testing.T) {
	source, err := os.ReadFile("native.cpp")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	chatTemplateAt := strings.Index(text, "llama_model_chat_template")
	applyAt := strings.Index(text, "llama_chat_apply_template")
	tokenizeAt := strings.Index(text, "llama_tokenize(")
	if chatTemplateAt < 0 || applyAt < 0 {
		t.Fatal("native generation must apply the model chat template")
	}
	if tokenizeAt < 0 || applyAt > tokenizeAt {
		t.Fatalf("chat template must be applied before tokenization: apply=%d tokenize=%d", applyAt, tokenizeAt)
	}
	if !strings.Contains(text[:tokenizeAt], "const struct llama_chat_message message = { \"user\", prompt };") {
		t.Fatal("chat template must receive one user message")
	}
	if !strings.Contains(text[applyAt:tokenizeAt], ", 1, true,") {
		t.Fatal("chat template must add the assistant generation prompt")
	}
}
