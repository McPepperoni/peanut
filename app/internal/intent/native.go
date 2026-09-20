package intent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"peanut/internal/llama"
)

const nativeMaxTokens = 512

type IntentParser interface {
	Parse(context.Context, string, CapabilitySnapshot) (ActionPlan, error)
}

type NativeParser struct {
	engine  llama.Engine
	threads int
	timeout time.Duration

	closeOnce sync.Once
	closeErr  error
}

func NewNativeParser(engine llama.Engine, threads int, timeout time.Duration) *NativeParser {
	return &NativeParser{engine: engine, threads: threads, timeout: timeout}
}

func (p *NativeParser) Parse(ctx context.Context, transcript string, snapshot CapabilitySnapshot) (ActionPlan, error) {
	if ctx == nil {
		return ActionPlan{}, errors.New("intent parse context is required")
	}
	if err := ValidateSnapshot(snapshot); err != nil {
		return ActionPlan{}, fmt.Errorf("invalid capability snapshot: %w", err)
	}
	prompt, err := renderPrompt(transcript, snapshot)
	if err != nil {
		return ActionPlan{}, err
	}
	if p.engine == nil {
		return ActionPlan{}, errors.New("intent native engine is unavailable")
	}
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	output, err := p.engine.Generate(ctx, llama.Request{
		Prompt:    prompt,
		Schema:    planJSONSchema,
		Threads:   p.threads,
		MaxTokens: nativeMaxTokens,
	})
	if err != nil {
		return ActionPlan{}, fmt.Errorf("run native intent model: %w", err)
	}
	data, err := extractJSONObject(output)
	if err != nil {
		return ActionPlan{}, err
	}
	plan, err := DecodePlan(data)
	if err != nil {
		return ActionPlan{}, fmt.Errorf("parse intent model JSON: %w", err)
	}
	if err := ValidatePlan(plan, snapshot); err != nil {
		return ActionPlan{}, fmt.Errorf("validate intent model plan: %w", err)
	}
	return plan, nil
}

func (p *NativeParser) Close() error {
	if p == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		if p.engine != nil {
			p.closeErr = p.engine.Close()
		}
	})
	return p.closeErr
}

func extractJSONObject(output []byte) ([]byte, error) {
	start := bytes.IndexByte(output, '{')
	if start < 0 {
		return nil, fmt.Errorf("intent model output contains no JSON object")
	}
	depth, inString, escaped := 0, false, false
	for i := start; i < len(output); i++ {
		switch {
		case escaped:
			escaped = false
		case inString && output[i] == '\\':
			escaped = true
		case output[i] == '"':
			inString = !inString
		case !inString && output[i] == '{':
			depth++
		case !inString && output[i] == '}':
			depth--
			if depth == 0 {
				return output[start : i+1], nil
			}
		}
	}
	return nil, fmt.Errorf("intent model output contains incomplete JSON object")
}

var _ IntentParser = (*NativeParser)(nil)
var _ io.Closer = (*NativeParser)(nil)
