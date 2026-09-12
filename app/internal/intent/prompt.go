package intent

import (
	"encoding/json"
	"fmt"
)

const planJSONSchema = `{"type":"object","properties":{"version":{"type":"integer","const":1},"status":{"type":"string","enum":["execute","clarify","unknown"]},"language":{"type":"string","minLength":1},"steps":{"type":"array","maxItems":16,"items":{"type":"object","properties":{"device_id":{"type":"string"},"action_id":{"type":"string"},"arguments":{"type":"object","additionalProperties":true}},"required":["device_id","action_id","arguments"],"additionalProperties":false}},"clarification":{"type":"string"},"confidence":{"type":"number","minimum":0,"maximum":1}},"required":["version","status","language","steps","clarification","confidence"],"additionalProperties":false}`

func renderPrompt(transcript string, snapshot CapabilitySnapshot) (string, error) {
	capabilities, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("encode capability snapshot: %w", err)
	}
	quotedTranscript, err := json.Marshal(transcript)
	if err != nil {
		return "", fmt.Errorf("encode transcript: %w", err)
	}
	return fmt.Sprintf(`Convert the transcript into one JSON intent plan. Output JSON only; never answer the user.
Use only device_id, action_id, and arguments present in the capability snapshot. Never invent capabilities.
Use status "execute" for supported unambiguous actions, "clarify" when user input is required, or "unknown" when unsupported.
Preserve requested step order. Set version to 1, language to the transcript language, and confidence from 0 to 1.
Capability snapshot: %s
Transcript: %s`, capabilities, quotedTranscript), nil
}
