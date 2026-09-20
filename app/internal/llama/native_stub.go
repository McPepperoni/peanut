//go:build !cgo || !peanut_llama

package llama

import (
	"context"
	"fmt"
)

type stubEngine struct{}

func Open(ctx context.Context, modelPath string, threads int) (Engine, error) {
	if err := validateOpen(ctx, modelPath, threads); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%w: build with cgo and peanut_llama", ErrUnavailable)
}

func (*stubEngine) Generate(ctx context.Context, request Request) ([]byte, error) {
	if err := validateRequest(request); err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is required", ErrInvalidRequest)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%w: build with cgo and peanut_llama", ErrUnavailable)
}

func (*stubEngine) Close() error { return nil }
