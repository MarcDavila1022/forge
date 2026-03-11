package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type slsaVerifyFixture struct {
	ForgeArgs              []string                `json:"forge_args"`
	ExpectedExitCode       int                     `json:"expected_exit_code"`
	ExpectedLevel          *int                    `json:"expected_level"`
	OutputContains         []string                `json:"output_contains"`
	OutputNotContains      []string                `json:"output_not_contains"`
	GHArgsContains         []string                `json:"gh_args_contains"`
	GHArgsNotContains      []string                `json:"gh_args_not_contains"`
	ExpectGHInvoked        *bool                   `json:"expect_gh_invoked"`
	JSONVerificationResult string                  `json:"json_verification_result"`
	JSONVerifiedLevels     []string                `json:"json_verified_levels"`
	JSONChecks             []slsaVerifyCheckExpect `json:"json_checks"`
}

type slsaVerifyCheckExpect struct {
	Name           string `json:"name"`
	Level          string `json:"level"`
	Result         string `json:"result"`
	DetailContains string `json:"detail_contains"`
}

type slsaVerifyJSONStatement struct {
	Predicate slsaVerifyJSONPredicate `json:"predicate"`
}

type slsaVerifyJSONPredicate struct {
	VerificationResult string                  `json:"verificationResult"`
	VerifiedLevels     []string                `json:"verifiedLevels"`
	ForgeExtensions    slsaVerifyJSONExtension `json:"https://forge.dev/extensions/v1"`
}

type slsaVerifyJSONExtension struct {
	BuildTrackChecks []slsaVerifyJSONCheck `json:"buildTrackChecks"`
}

type slsaVerifyJSONCheck struct {
	Name   string `json:"name"`
	Level  string `json:"level"`
	Result string `json:"result"`
	Detail string `json:"detail"`
}

func TestSLSAVerifyFixtures(t *testing.T) {
	fixturesRoot := filepath.Join(repoRoot(t), "tests", "fixtures", "slsa-verify")
	entries, err := os.ReadDir(fixturesRoot)
	if err != nil {
		t.Fatalf("read verify fixtures: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "common" {
			continue
		}

		name := entry.Name()
		caseDir := filepath.Join(fixturesRoot, name)
		t.Run(name, func(t *testing.T) {
			fixture := loadSLSAVerifyFixture(t, caseDir)
			tmpDir := t.TempDir()
			copyDir(t, filepath.Join(fixturesRoot, "common"), tmpDir)
			if _, err := os.Stat(filepath.Join(caseDir, "repo")); err == nil {
				copyDir(t, filepath.Join(caseDir, "repo"), tmpDir)
			}

			binDir := t.TempDir()
			argsFile := filepath.Join(binDir, "gh-args.txt")
			writeExecutable(t, binDir, "gh", `#!/bin/sh
set -eu

if [ "${1:-}" = "--version" ]; then
  if [ -n "${FORGE_GH_VERSION_FILE:-}" ] && [ -f "$FORGE_GH_VERSION_FILE" ]; then
    cat "$FORGE_GH_VERSION_FILE"
  else
    printf 'gh version 0.0.0\n'
  fi
  exit 0
fi

printf '%s\n' "$@" > "$FORGE_GH_ARGS_FILE"
if [ -n "${FORGE_GH_STDOUT_FILE:-}" ] && [ -f "$FORGE_GH_STDOUT_FILE" ]; then
  cat "$FORGE_GH_STDOUT_FILE"
fi
if [ -n "${FORGE_GH_STDERR_FILE:-}" ] && [ -f "$FORGE_GH_STDERR_FILE" ]; then
  cat "$FORGE_GH_STDERR_FILE" >&2
fi
code=0
if [ -n "${FORGE_GH_EXITCODE_FILE:-}" ] && [ -f "$FORGE_GH_EXITCODE_FILE" ]; then
  code=$(tr -d ' \n\r\t' < "$FORGE_GH_EXITCODE_FILE")
fi
exit "$code"
`)

			result := runForge(t, tmpDir, map[string]string{
				"PATH":                   prependPATH(binDir),
				"FORGE_GH_ARGS_FILE":     argsFile,
				"FORGE_GH_STDOUT_FILE":   filepath.Join(caseDir, "gh.stdout"),
				"FORGE_GH_STDERR_FILE":   filepath.Join(caseDir, "gh.stderr"),
				"FORGE_GH_EXITCODE_FILE": filepath.Join(caseDir, "gh.exitcode"),
				"FORGE_GH_VERSION_FILE":  filepath.Join(caseDir, "gh.version"),
			}, fixture.ForgeArgs...)

			expectedExitCode := fixture.ExpectedExitCode
			if result.exitCode != expectedExitCode {
				t.Fatalf("expected exit code %d, got %d with output:\n%s", expectedExitCode, result.exitCode, result.output)
			}

			if fixture.ExpectedLevel != nil {
				levelText := fmt.Sprintf("Verified SLSA Build Level: L%d", *fixture.ExpectedLevel)
				if !strings.Contains(result.output, levelText) {
					t.Fatalf("expected %q in output, got:\n%s", levelText, result.output)
				}
			}
			for _, expected := range fixture.OutputContains {
				if !strings.Contains(result.output, expected) {
					t.Fatalf("expected %q in output, got:\n%s", expected, result.output)
				}
			}
			for _, unexpected := range fixture.OutputNotContains {
				if strings.Contains(result.output, unexpected) {
					t.Fatalf("did not expect %q in output, got:\n%s", unexpected, result.output)
				}
			}

			expectGHInvoked := true
			if fixture.ExpectGHInvoked != nil {
				expectGHInvoked = *fixture.ExpectGHInvoked
			}
			if !expectGHInvoked {
				if _, err := os.Stat(argsFile); !os.IsNotExist(err) {
					t.Fatalf("expected gh not to be invoked, args file exists")
				}
				return
			}

			argsContent, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatalf("read gh args file: %v", err)
			}
			argsText := string(argsContent)
			for _, expected := range fixture.GHArgsContains {
				if !strings.Contains(argsText, expected) {
					t.Fatalf("expected %q in gh args, got:\n%s", expected, argsText)
				}
			}
			for _, unexpected := range fixture.GHArgsNotContains {
				if strings.Contains(argsText, unexpected) {
					t.Fatalf("did not expect %q in gh args, got:\n%s", unexpected, argsText)
				}
			}

			if fixture.JSONVerificationResult != "" || len(fixture.JSONVerifiedLevels) > 0 || len(fixture.JSONChecks) > 0 {
				jsonResult := runForge(t, tmpDir, map[string]string{
					"PATH":                   prependPATH(binDir),
					"FORGE_GH_ARGS_FILE":     argsFile,
					"FORGE_GH_STDOUT_FILE":   filepath.Join(caseDir, "gh.stdout"),
					"FORGE_GH_STDERR_FILE":   filepath.Join(caseDir, "gh.stderr"),
					"FORGE_GH_EXITCODE_FILE": filepath.Join(caseDir, "gh.exitcode"),
					"FORGE_GH_VERSION_FILE":  filepath.Join(caseDir, "gh.version"),
				}, withOutputJSON(fixture.ForgeArgs)...)
				if jsonResult.exitCode != expectedExitCode {
					t.Fatalf("expected json run exit code %d, got %d with output:\n%s", expectedExitCode, jsonResult.exitCode, jsonResult.output)
				}
				assertSLSAVerifyJSON(t, jsonResult.output, fixture)
			}
		})
	}
}

func loadSLSAVerifyFixture(t *testing.T, caseDir string) slsaVerifyFixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(caseDir, "fixture.json"))
	if err != nil {
		t.Fatalf("read fixture %s: %v", caseDir, err)
	}
	var fixture slsaVerifyFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("parse fixture %s: %v", caseDir, err)
	}
	return fixture
}

func withOutputJSON(args []string) []string {
	jsonArgs := make([]string, 0, len(args)+2)
	jsonArgs = append(jsonArgs, args...)
	return append(jsonArgs, "--output", "json")
}

func assertSLSAVerifyJSON(t *testing.T, output string, fixture slsaVerifyFixture) {
	t.Helper()
	var stmt slsaVerifyJSONStatement
	if err := json.Unmarshal([]byte(output), &stmt); err != nil {
		t.Fatalf("parse verify json output: %v\n%s", err, output)
	}

	if fixture.JSONVerificationResult != "" && stmt.Predicate.VerificationResult != fixture.JSONVerificationResult {
		t.Fatalf("expected verificationResult %q, got %q", fixture.JSONVerificationResult, stmt.Predicate.VerificationResult)
	}

	if len(fixture.JSONVerifiedLevels) > 0 {
		for _, expected := range fixture.JSONVerifiedLevels {
			if !containsString(stmt.Predicate.VerifiedLevels, expected) {
				t.Fatalf("expected verified level %q, got %#v", expected, stmt.Predicate.VerifiedLevels)
			}
		}
	}

	for _, expected := range fixture.JSONChecks {
		check, ok := findVerifyJSONCheck(stmt.Predicate.ForgeExtensions.BuildTrackChecks, expected.Name)
		if !ok {
			t.Fatalf("expected check %q in JSON output", expected.Name)
		}
		if expected.Level != "" && check.Level != expected.Level {
			t.Fatalf("expected check %q level %q, got %q", expected.Name, expected.Level, check.Level)
		}
		if expected.Result != "" && check.Result != expected.Result {
			t.Fatalf("expected check %q result %q, got %q", expected.Name, expected.Result, check.Result)
		}
		if expected.DetailContains != "" && !strings.Contains(check.Detail, expected.DetailContains) {
			t.Fatalf("expected check %q detail to contain %q, got %q", expected.Name, expected.DetailContains, check.Detail)
		}
	}
}

func findVerifyJSONCheck(checks []slsaVerifyJSONCheck, name string) (slsaVerifyJSONCheck, bool) {
	for _, check := range checks {
		if check.Name == name {
			return check, true
		}
	}
	return slsaVerifyJSONCheck{}, false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
