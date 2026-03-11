package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitL3UsesLocalReusableWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	result := runForge(t, tmpDir, nil, "init", "--l3")
	if result.exitCode != 0 {
		t.Fatalf("init --l3 failed: %s", result.output)
	}

	workflow, err := os.ReadFile(filepath.Join(tmpDir, ".github/workflows/secure-release.yml"))
	if err != nil {
		t.Fatalf("read generated workflow: %v", err)
	}
	if !strings.Contains(string(workflow), "uses: ./.github/workflows/build-and-attest.yml") {
		t.Fatalf("expected local reusable workflow path, got:\n%s", string(workflow))
	}
	if !strings.Contains(result.output, "Reusable workflow release scaffolding generated.") {
		t.Fatalf("expected updated init messaging, got:\n%s", result.output)
	}
}
