package intent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func DecodePlan(data []byte) (ActionPlan, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire struct {
		Version       *int             `json:"version"`
		Status        *PlanStatus      `json:"status"`
		Language      *string          `json:"language"`
		Steps         *[]ActionRequest `json:"steps"`
		Clarification *string          `json:"clarification"`
		Confidence    *float64         `json:"confidence"`
	}
	if err := decoder.Decode(&wire); err != nil {
		return ActionPlan{}, fmt.Errorf("decode plan: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return ActionPlan{}, fmt.Errorf("decode plan: %w", err)
	}
	if wire.Version == nil || wire.Status == nil || wire.Language == nil || wire.Steps == nil || wire.Clarification == nil || wire.Confidence == nil {
		return ActionPlan{}, fmt.Errorf("decode plan: all envelope fields are required")
	}
	return ActionPlan{
		Version: *wire.Version, Status: *wire.Status, Language: *wire.Language, Steps: *wire.Steps,
		Clarification: *wire.Clarification, Confidence: *wire.Confidence,
	}, nil
}
