package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLlamaSetupScripts(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"tools/setup-llama.sh", "tools/setup-llama.ps1"} {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "git submodule update --init --recursive third-party/llama.cpp") {
			t.Fatalf("%s does not initialize pinned submodule", path)
		}
		if strings.Contains(text, "--remote") || strings.Contains(text, "git pull") || strings.Contains(text, "curl") || strings.Contains(text, "wget") {
			t.Fatalf("%s performs unpinned setup: %q", path, text)
		}
	}

	gitmodules, err := os.ReadFile(filepath.Join(root, ".gitmodules"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gitmodules), "third-party/llama.cpp") || !strings.Contains(string(gitmodules), "https://github.com/ggml-org/llama.cpp.git") {
		t.Fatalf("unexpected .gitmodules: %q", gitmodules)
	}
}

func TestLlamaDocumentationContract(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	release, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	releaseText := string(release)
	if !strings.Contains(releaseText, "./tools/build-llama.sh") {
		t.Fatal("release workflow does not use the pinned native llama build helper")
	}
	buildHelper, err := os.ReadFile(filepath.Join(root, "tools", "build-llama.sh"))
	if err != nil {
		t.Fatal(err)
	}
	buildHelperText := string(buildHelper)
	for _, required := range []string{
		"-DGGML_OPENMP=OFF",
		"-DLLAMA_OPENSSL=OFF",
		"-DLLAMA_SUBPROCESS=OFF",
	} {
		if !strings.Contains(buildHelperText, required) {
			t.Fatalf("native llama build helper does not contain %q", required)
		}
	}
	paths := []string{
		"README.md",
		"tools/llama.cpp.md",
		"tools/models.md",
		"docs/pi5-validation.md",
	}
	var docs strings.Builder
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		docs.Write(data)
	}
	text := docs.String()
	for _, required := range []string{
		"a894dae939d426954ce54bb604824f1ae918a0c5",
		"git submodule update --init --recursive third-party/llama.cpp",
		"/var/lib/peanut/models/<model-name>.gguf",
		"Models.IntentModel=functiongemma.gguf",
		"The intent GGUF is a single external flat file:",
		"five Sherpa roles",
		"CGO_ENABLED=1",
		"peanut_llama",
		"linux/amd64",
		"linux/arm64",
		"windows/amd64",
		"darwin/arm64",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("documentation does not contain %q", required)
		}
	}
	if strings.Contains(strings.ToLower(text), "llama-cli") {
		t.Fatal("documentation still describes llama-cli inference")
	}
	piValidation, err := os.ReadFile(filepath.Join(root, "docs", "pi5-validation.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(piValidation), "all six active roles") {
		t.Fatal("Pi 5 documentation still describes intent as a sixth profile role")
	}
}

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
