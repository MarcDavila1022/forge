package tests

import (
	"strings"
	"testing"
)

func TestHardenVerifyRejectsMutableExternalRefs(t *testing.T) {
	tmpDir := t.TempDir()
	writeFile(t, tmpDir, ".github/workflows/secure-release.yml", `name: Secure Release
jobs:
  release:
    uses: owner/repo/.github/workflows/build-and-attest.yml@main
`)

	result := runForge(t, tmpDir, nil, "harden", "--verify")
	if result.exitCode != 1 {
		t.Fatalf("expected harden --verify to fail, got exit code %d with output:\n%s", result.exitCode, result.output)
	}
	if !strings.Contains(result.output, "mutable external ref: owner/repo/.github/workflows/build-and-attest.yml@main") {
		t.Fatalf("expected mutable external ref in output, got:\n%s", result.output)
	}
}
