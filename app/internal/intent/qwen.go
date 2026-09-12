package intent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type IntentParser interface {
	Parse(ctx context.Context, transcript string, snapshot CapabilitySnapshot) (ActionPlan, error)
}

type inferenceRunner interface {
	Run(ctx context.Context, executable string, args []string) ([]byte, error)
}

type QwenParser struct {
	executable string
	modelPath  string
	threads    int
	timeout    time.Duration
	runner     inferenceRunner
}

func NewQwenParser(executable, modelPath string, threads int, timeout time.Duration) *QwenParser {
	return newQwenParser(executable, modelPath, threads, timeout, commandRunner{})
}

func newQwenParser(executable, modelPath string, threads int, timeout time.Duration, runner inferenceRunner) *QwenParser {
	return &QwenParser{executable: executable, modelPath: modelPath, threads: threads, timeout: timeout, runner: runner}
}

func (p *QwenParser) Parse(ctx context.Context, transcript string, snapshot CapabilitySnapshot) (ActionPlan, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return ActionPlan{}, fmt.Errorf("invalid capability snapshot: %w", err)
	}
	prompt, err := renderPrompt(transcript, snapshot)
	if err != nil {
		return ActionPlan{}, err
	}
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	output, err := p.runner.Run(ctx, p.executable, []string{
		"--model", p.modelPath,
		"--threads", strconv.Itoa(p.threads),
		"--threads-batch", strconv.Itoa(p.threads),
		"--device", "none",
		"--seed", "1",
		"--temp", "0",
		"--reasoning", "off",
		"--chat-template-kwargs", `{"enable_thinking":false}`,
		"--jinja",
		"--single-turn",
		"--json-schema", planJSONSchema,
		"--prompt", prompt,
		"--no-display-prompt",
		"--no-show-timings",
		"--simple-io",
		"--offline",
		"--n-predict", "512",
	})
	if err != nil {
		return ActionPlan{}, fmt.Errorf("run Qwen inference: %w", err)
	}
	data, err := extractJSONObject(output)
	if err != nil {
		return ActionPlan{}, err
	}
	plan, err := DecodePlan(data)
	if err != nil {
		return ActionPlan{}, fmt.Errorf("parse Qwen JSON: %w", err)
	}
	if err := ValidatePlan(plan, snapshot); err != nil {
		return ActionPlan{}, fmt.Errorf("validate Qwen plan: %w", err)
	}
	return plan, nil
}

func extractJSONObject(output []byte) ([]byte, error) {
	start := bytes.IndexByte(output, '{')
	if start < 0 {
		return nil, fmt.Errorf("Qwen output contains no JSON object")
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
	return nil, fmt.Errorf("Qwen output contains incomplete JSON object")
}

type commandRunner struct{}

func (commandRunner) Run(ctx context.Context, executable string, args []string) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, args...)
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return nil, fmt.Errorf("%w: %s", err, detail)
		}
		return nil, err
	}
	return output, nil
}

var _ IntentParser = (*QwenParser)(nil)
