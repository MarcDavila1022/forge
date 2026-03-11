package tests

import (
	"strings"
	"testing"
)

func TestSLSAStaticReportsEstimatedLevel(t *testing.T) {
	tmpDir := t.TempDir()
	writeFile(t, tmpDir, ".github/workflows/build-and-attest.yml", reusableWorkflow())
	writeFile(t, tmpDir, ".github/workflows/secure-release.yml", `name: Secure Release
jobs:
  release:
    uses: ./.github/workflows/build-and-attest.yml
    with:
      version: ${{ github.ref_name }}
`)

	result := runForge(t, tmpDir, nil, "slsa", "--workflow", ".github/workflows/secure-release.yml")
	if result.exitCode != 0 {
		t.Fatalf("slsa failed: %s", result.output)
	}
	if !strings.Contains(result.output, "Estimated SLSA Build Level: L3") {
		t.Fatalf("expected estimated level output, got:\n%s", result.output)
	}
	if !strings.Contains(result.output, "reusable_workflow_reference_immutable") {
		t.Fatalf("expected immutable reusable workflow check in output, got:\n%s", result.output)
	}
}
