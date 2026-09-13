package install

import (
	"os"
	"strings"
	"testing"
)

func TestProductionInstallersKeepHomeAssistantExternal(t *testing.T) {
	for _, name := range []string{"install.sh", "install.ps1"} {
		t.Run(name, func(t *testing.T) {
			contents, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			script := string(contents)
			for _, required := range []string{
				"Do you already have Home Assistant?",
				"ghcr.io/home-assistant/home-assistant:stable",
				"/api/v1/config",
				"home_assistant",
				"go build",
			} {
				if !strings.Contains(script, required) {
					t.Errorf("%s does not contain %q", name, required)
				}
			}
			lower := strings.ToLower(script)
			if strings.Contains(lower, "git clone") {
				t.Errorf("%s clones Home Assistant source", name)
			}
			if !strings.Contains(lower, "docker") || !strings.Contains(lower, "podman") {
				t.Errorf("%s does not detect Docker and Podman", name)
			}
		})
	}
}
