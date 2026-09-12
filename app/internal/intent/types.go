package intent

const MaxPlanSteps = 16

type PlanStatus string

const (
	StatusExecute PlanStatus = "execute"
	StatusClarify PlanStatus = "clarify"
	StatusUnknown PlanStatus = "unknown"
)

type JSONType string

const (
	TypeString  JSONType = "string"
	TypeNumber  JSONType = "number"
	TypeInteger JSONType = "integer"
	TypeBoolean JSONType = "boolean"
	TypeObject  JSONType = "object"
	TypeArray   JSONType = "array"
	TypeNull    JSONType = "null"
)

type ArgumentDefinition struct {
	Type     JSONType `json:"type"`
	Required bool     `json:"required,omitempty"`
}

type ActionDefinition struct {
	ID        string                        `json:"id"`
	Arguments map[string]ArgumentDefinition `json:"arguments,omitempty"`
}

type Capability struct {
	ID         string             `json:"id"`
	ProviderID string             `json:"provider_id"`
	DeviceID   string             `json:"device_id"`
	Type       string             `json:"type"`
	Name       string             `json:"name"`
	Room       string             `json:"room,omitempty"`
	Actions    []ActionDefinition `json:"actions"`
}

type CapabilitySnapshot struct {
	Capabilities []Capability `json:"capabilities"`
}

type ActionRequest struct {
	DeviceID  string         `json:"device_id"`
	ActionID  string         `json:"action_id"`
	Arguments map[string]any `json:"arguments"`
}

type ActionPlan struct {
	Version       int             `json:"version"`
	Status        PlanStatus      `json:"status"`
	Language      string          `json:"language"`
	Steps         []ActionRequest `json:"steps"`
	Clarification string          `json:"clarification"`
	Confidence    float64         `json:"confidence"`
}
