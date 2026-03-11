package tests

import (
	"strings"
	"testing"
)

func TestScanReportsGovulncheckFindings(t *testing.T) {
	tmpDir := t.TempDir()
	writeFile(t, tmpDir, "go.mod", "module example.com/test\n\ngo 1.25.0\n")

	binDir := t.TempDir()
	writeExecutable(t, binDir, "govulncheck", `#!/bin/sh
printf '%s\n' '{"finding":{"osv":"GO-TEST-0001","trace":[{"module":"example.com/mod","version":"v1.0.0"}]}}'
`)

	result := runForge(t, tmpDir, map[string]string{
		"PATH": prependPATH(binDir),
	}, "scan", "--severity", "unknown")
	if result.exitCode != 1 {
		t.Fatalf("expected scan to fail for reported finding, got exit code %d with output:\n%s", result.exitCode, result.output)
	}
	if !strings.Contains(result.output, "GO-TEST-0001") {
		t.Fatalf("expected scan output to include finding ID, got:\n%s", result.output)
	}
	if !strings.Contains(result.output, "UNKNOWN") {
		t.Fatalf("expected scan output to show fallback severity, got:\n%s", result.output)
	}
}
