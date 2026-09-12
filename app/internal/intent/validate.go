package intent

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

func ValidatePlan(plan ActionPlan, snapshot CapabilitySnapshot) error {
	if err := ValidateSnapshot(snapshot); err != nil {
		return fmt.Errorf("invalid capability snapshot: %w", err)
	}
	if plan.Version != 1 {
		return fmt.Errorf("unsupported plan version %d", plan.Version)
	}
	if strings.TrimSpace(plan.Language) == "" {
		return errors.New("plan language is required")
	}
	if math.IsNaN(plan.Confidence) || plan.Confidence < 0 || plan.Confidence > 1 {
		return errors.New("plan confidence must be between 0 and 1")
	}
	switch plan.Status {
	case StatusExecute:
		if len(plan.Steps) == 0 {
			return errors.New("execute plan requires at least one step")
		}
		if len(plan.Steps) > MaxPlanSteps {
			return fmt.Errorf("plan exceeds %d steps", MaxPlanSteps)
		}
	case StatusClarify:
		if len(plan.Steps) != 0 {
			return errors.New("clarify plan cannot contain steps")
		}
		if strings.TrimSpace(plan.Clarification) == "" {
			return errors.New("clarify plan requires clarification")
		}
	case StatusUnknown:
		if len(plan.Steps) != 0 {
			return errors.New("unknown plan cannot contain steps")
		}
	default:
		return fmt.Errorf("invalid plan status %q", plan.Status)
	}
	for i, step := range plan.Steps {
		if err := validateStep(step, snapshot); err != nil {
			return fmt.Errorf("step %d: %w", i, err)
		}
	}
	return nil
}

func ValidateSnapshot(snapshot CapabilitySnapshot) error {
	capabilityIDs := make(map[string]bool)
	deviceActions := make(map[string]map[string]bool)
	for _, capability := range snapshot.Capabilities {
		if capability.ID == "" || capability.ProviderID == "" || capability.DeviceID == "" || capability.Type == "" || capability.Name == "" {
			return errors.New("capability stable IDs, type, and name are required")
		}
		if capabilityIDs[capability.ID] {
			return fmt.Errorf("duplicate capability ID %q", capability.ID)
		}
		capabilityIDs[capability.ID] = true
		if deviceActions[capability.DeviceID] == nil {
			deviceActions[capability.DeviceID] = make(map[string]bool)
		}
		for _, action := range capability.Actions {
			if action.ID == "" {
				return errors.New("action stable ID is required")
			}
			if deviceActions[capability.DeviceID][action.ID] {
				return fmt.Errorf("duplicate action ID %q for device %q", action.ID, capability.DeviceID)
			}
			deviceActions[capability.DeviceID][action.ID] = true
			for name, definition := range action.Arguments {
				if name == "" || !validJSONType(definition.Type) {
					return fmt.Errorf("invalid argument schema for action %q", action.ID)
				}
			}
			for _, name := range action.ExactlyOneOf {
				if _, ok := action.Arguments[name]; !ok {
					return fmt.Errorf("exactly-one argument %q is not declared for action %q", name, action.ID)
				}
			}
		}
	}
	return nil
}

func validateStep(step ActionRequest, snapshot CapabilitySnapshot) error {
	deviceFound := false
	for _, capability := range snapshot.Capabilities {
		if capability.DeviceID != step.DeviceID {
			continue
		}
		deviceFound = true
		for _, action := range capability.Actions {
			if action.ID == step.ActionID {
				return validateArguments(step.Arguments, action.Arguments, action.ExactlyOneOf)
			}
		}
	}
	if !deviceFound {
		return fmt.Errorf("unknown device %q", step.DeviceID)
	}
	return fmt.Errorf("unknown action %q for device %q", step.ActionID, step.DeviceID)
}

func validateArguments(arguments map[string]any, schema map[string]ArgumentDefinition, exactlyOneOf []string) error {
	for name, definition := range schema {
		value, ok := arguments[name]
		if definition.Required && !ok {
			return fmt.Errorf("required argument %s is missing", name)
		}
		if ok && !matchesJSONType(value, definition.Type) {
			return fmt.Errorf("argument %s must be %s", name, definition.Type)
		}
	}
	for name := range arguments {
		if _, ok := schema[name]; !ok {
			return fmt.Errorf("unknown argument %s", name)
		}
	}
	present := 0
	for _, name := range exactlyOneOf {
		if _, ok := arguments[name]; ok {
			present++
		}
	}
	if len(exactlyOneOf) > 0 && present != 1 {
		return fmt.Errorf("exactly one of %s is required", strings.Join(exactlyOneOf, ", "))
	}
	return nil
}

func validJSONType(kind JSONType) bool {
	switch kind {
	case TypeString, TypeNumber, TypeInteger, TypeBoolean, TypeObject, TypeArray, TypeNull:
		return true
	default:
		return false
	}
}

func matchesJSONType(value any, kind JSONType) bool {
	switch kind {
	case TypeString:
		_, ok := value.(string)
		return ok
	case TypeBoolean:
		_, ok := value.(bool)
		return ok
	case TypeObject:
		_, ok := value.(map[string]any)
		return ok
	case TypeArray:
		_, ok := value.([]any)
		return ok
	case TypeNull:
		return value == nil
	case TypeNumber:
		return finiteNumber(value, false)
	case TypeInteger:
		return finiteNumber(value, true)
	default:
		return false
	}
}

func finiteNumber(value any, integer bool) bool {
	var number float64
	switch value := value.(type) {
	case int:
		return true
	case int8:
		return true
	case int16:
		return true
	case int32:
		return true
	case int64:
		return true
	case uint:
		return true
	case uint8:
		return true
	case uint16:
		return true
	case uint32:
		return true
	case uint64:
		return true
	case float32:
		number = float64(value)
	case float64:
		number = value
	default:
		return false
	}
	return !math.IsNaN(number) && !math.IsInf(number, 0) && (!integer || math.Trunc(number) == number)
}
