package llama

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnavailable    = errors.New("llama unavailable")
	ErrInvalidRequest = errors.New("invalid llama request")
	ErrNativeLoad     = errors.New("llama model load failed")
	ErrNativeContext  = errors.New("llama context creation failed")
	ErrNativeGrammar  = errors.New("llama grammar creation failed")
	ErrNativeTokenize = errors.New("llama prompt tokenization failed")
	ErrNativeDecode   = errors.New("llama decode failed")
	ErrNativeOutput   = errors.New("llama output failed")
	ErrCancelled      = errors.New("llama generation cancelled")
)

const maxTokens = 4096

type Request struct {
	Prompt    string
	Schema    string
	Threads   int
	MaxTokens int
}

type Engine interface {
	Generate(context.Context, Request) ([]byte, error)
	Close() error
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.Prompt) == "" {
		return fmt.Errorf("%w: prompt is required", ErrInvalidRequest)
	}
	if request.Threads <= 0 {
		return fmt.Errorf("%w: threads must be positive", ErrInvalidRequest)
	}
	if request.MaxTokens <= 0 || request.MaxTokens > maxTokens {
		return fmt.Errorf("%w: max tokens must be between 1 and %d", ErrInvalidRequest, maxTokens)
	}
	return nil
}

func validateOpen(ctx context.Context, modelPath string, threads int) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(modelPath) == "" {
		return fmt.Errorf("%w: model path is required", ErrInvalidRequest)
	}
	if threads <= 0 {
		return fmt.Errorf("%w: threads must be positive", ErrInvalidRequest)
	}
	return ctx.Err()
}
