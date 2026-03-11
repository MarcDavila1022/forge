package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	buildOnce   sync.Once
	forgeBinary string
	buildErr    error
)

type runResult struct {
	output   string
	exitCode int
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, ".."))
}

func buildForgeBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "forge-bin-*")
		if err != nil {
			buildErr = err
			return
		}
		forgeBinary = filepath.Join(tmpDir, "forge")
		if runtime.GOOS == "windows" {
			forgeBinary += ".exe"
		}

		cmd := exec.Command("go", "build", "-o", forgeBinary, ".")
		cmd.Dir = repoRoot(t)
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("go build failed: %w\n%s", err, string(out))
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return forgeBinary
}

func mergedEnv(overrides map[string]string) []string {
	envMap := map[string]string{}
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		key := parts[0]
		value := ""
		if len(parts) == 2 {
			value = parts[1]
		}
		envMap[key] = value
	}
	for key, value := range overrides {
		envMap[key] = value
	}
	env := make([]string, 0, len(envMap))
	for key, value := range envMap {
		env = append(env, key+"="+value)
	}
	return env
}

func runCommand(t *testing.T, dir string, env map[string]string, name string, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = mergedEnv(env)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return runResult{output: string(out)}
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("run %s %v: %v\n%s", name, args, err, string(out))
	}
	return runResult{output: string(out), exitCode: exitErr.ExitCode()}
}

func runForge(t *testing.T, dir string, env map[string]string, args ...string) runResult {
	t.Helper()
	return runCommand(t, dir, env, buildForgeBinary(t), args...)
}

func writeFile(t *testing.T, root string, rel string, content string) {
	t.Helper()
	fullPath := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func copyDir(t *testing.T, src string, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
	if err != nil {
		t.Fatalf("copy %s to %s: %v", src, dst, err)
	}
}

func writeExecutable(t *testing.T, root string, name string, content string) string {
	t.Helper()
	fullPath := filepath.Join(root, name)
	if err := os.WriteFile(fullPath, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", name, err)
	}
	return fullPath
}

func prependPATH(dir string) string {
	return dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	result := runCommand(t, dir, nil, "git", "init")
	if result.exitCode != 0 {
		t.Fatalf("git init failed: %s", result.output)
	}
	result = runCommand(t, dir, nil, "git", "remote", "add", "origin", "git@github.com:owner/repo.git")
	if result.exitCode != 0 {
		t.Fatalf("git remote add origin failed: %s", result.output)
	}
}

func reusableWorkflow() string {
	return `name: Build and Attest
on:
  workflow_call:
    inputs:
      version:
        required: true
        type: string
permissions:
  id-token: write
  attestations: write
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5
      - run: echo build
      - uses: actions/attest-build-provenance@e8998f949152b193b063cb0ec769d69d929409be
        with:
          subject-path: dist/*
`
}
