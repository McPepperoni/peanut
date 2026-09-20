//go:build cgo && peanut_llama

package llama

/*
#cgo CFLAGS: -I${SRCDIR}/../../../third-party/llama.cpp/include -I${SRCDIR}/../../../third-party/llama.cpp/ggml/include
#cgo linux LDFLAGS: -llama -lggml -lggml-cpu -lggml-base -lstdc++ -lm -ldl -pthread
#cgo darwin LDFLAGS: -llama -lggml -lggml-cpu -lggml-base -lc++ -lm
#cgo windows LDFLAGS: -llama -lggml -lggml-cpu -lggml-base
#include <stdlib.h>
#include "native.h"
*/
import "C"

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

var nativeBackend sync.Once

type nativeEngine struct {
	mu     sync.Mutex
	native *C.peanut_llama
}

func Open(ctx context.Context, modelPath string, threads int) (Engine, error) {
	if err := validateOpen(ctx, modelPath, threads); err != nil {
		return nil, err
	}
	nativeBackend.Do(func() { C.peanut_llama_backend_init() })

	cModelPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cModelPath))
	var native *C.peanut_llama
	status := C.peanut_llama_open(cModelPath, C.int32_t(threads), &native)
	if status != C.PEANUT_LLAMA_OK {
		return nil, nativeError(status)
	}
	return &nativeEngine{native: native}, nil
}

func (e *nativeEngine) Generate(ctx context.Context, request Request) ([]byte, error) {
	if err := validateRequest(request); err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is required", ErrInvalidRequest)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.native == nil {
		return nil, fmt.Errorf("%w: engine is closed", ErrUnavailable)
	}

	cPrompt := C.CString(request.Prompt)
	cSchema := C.CString(request.Schema)
	defer C.free(unsafe.Pointer(cPrompt))
	defer C.free(unsafe.Pointer(cSchema))

	stop := make(chan struct{})
	done := make(chan struct{})
	go func(native *C.peanut_llama) {
		defer close(done)
		select {
		case <-ctx.Done():
			C.peanut_llama_abort(native)
		case <-stop:
		}
	}(e.native)

	var nativeOutput *C.uchar
	var nativeOutputLen C.size_t
	status := C.peanut_llama_generate(
		e.native,
		cPrompt,
		cSchema,
		C.int32_t(request.MaxTokens),
		(**C.uchar)(unsafe.Pointer(&nativeOutput)),
		&nativeOutputLen,
	)
	close(stop)
	<-done
	if err := ctx.Err(); err != nil {
		if nativeOutput != nil {
			C.peanut_llama_free_output(e.native)
		}
		return nil, err
	}
	if status != C.PEANUT_LLAMA_OK {
		if nativeOutput != nil {
			C.peanut_llama_free_output(e.native)
		}
		return nil, nativeError(status)
	}
	if nativeOutput == nil || nativeOutputLen == 0 {
		C.peanut_llama_free_output(e.native)
		return nil, ErrNativeOutput
	}
	result := append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(nativeOutput)), int(nativeOutputLen))...)
	runtime.KeepAlive(request)
	C.peanut_llama_free_output(e.native)
	return result, nil
}

func (e *nativeEngine) Close() error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.native != nil {
		C.peanut_llama_close(e.native)
		e.native = nil
	}
	return nil
}

func nativeError(status C.peanut_llama_status) error {
	switch status {
	case C.PEANUT_LLAMA_INVALID:
		return ErrInvalidRequest
	case C.PEANUT_LLAMA_LOAD_FAILED:
		return ErrNativeLoad
	case C.PEANUT_LLAMA_CONTEXT_FAILED:
		return ErrNativeContext
	case C.PEANUT_LLAMA_GRAMMAR_FAILED:
		return ErrNativeGrammar
	case C.PEANUT_LLAMA_TOKENIZE_FAILED:
		return ErrNativeTokenize
	case C.PEANUT_LLAMA_DECODE_FAILED:
		return ErrNativeDecode
	case C.PEANUT_LLAMA_CANCELLED:
		return ErrCancelled
	case C.PEANUT_LLAMA_OUTPUT_FAILED:
		return ErrNativeOutput
	default:
		return fmt.Errorf("native llama status %d", int(status))
	}
}
