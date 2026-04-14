package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildVigil(t *testing.T, dir string) string {
	t.Helper()
	buildPath := filepath.Join(dir, "vigil")
	// Build from module root (parent of test/ dir), output to temp dir
	moduleRoot := ".."
	cmd := exec.Command("go", "build", "-o", buildPath, "./cmd/vigil")
	cmd.Dir = moduleRoot
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build vigil: %v", err)
	}
	return buildPath
}

func TestVigilBasicUsage(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Build vigil (from module root)
	buildPath := buildVigil(t, tmpDir)

	// Create test project structure
	testProject := filepath.Join(tmpDir, "testproj")
	if err := os.MkdirAll(testProject, 0755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	// Create CLAUDE.md
	claudePath := filepath.Join(testProject, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte("# Test Project\n"), 0644); err != nil {
		t.Fatalf("Failed to create CLAUDE.md: %v", err)
	}

	// Create some files
	os.WriteFile(filepath.Join(testProject, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(testProject, "README.md"), []byte("# Test"), 0644)
	os.MkdirAll(filepath.Join(testProject, "internal"), 0755)
	os.WriteFile(filepath.Join(testProject, "internal", "helper.go"), []byte("package internal"), 0644)

	// Run vigil
	runCmd := exec.Command(buildPath, "-n")
	runCmd.Dir = testProject
	output, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run vigil: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	// Check for the new informative message
	if !strings.Contains(outputStr, "IMPORTANT: This tree shows files and directories") {
		t.Errorf("Output should contain IMPORTANT message, got:\n%s", outputStr)
	}
	// Check that tree contains expected files
	if !strings.Contains(outputStr, "main.go") {
		t.Errorf("Output should contain main.go, got:\n%s", outputStr)
	}
	if !strings.Contains(outputStr, "README.md") {
		t.Errorf("Output should contain README.md, got:\n%s", outputStr)
	}
	if !strings.Contains(outputStr, "internal/") {
		t.Errorf("Output should contain internal/, got:\n%s", outputStr)
	}
}

func TestVigilHelp(t *testing.T) {
	tmpDir := t.TempDir()
	buildPath := buildVigil(t, tmpDir)

	runCmd := exec.Command(buildPath, "--help")
	output, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run --help: %v", err)
	}

	if !strings.Contains(string(output), "Usage:") {
		t.Errorf("Help output should contain usage info")
	}
}

func TestVigilNoCLAUDEmd(t *testing.T) {
	tmpDir := t.TempDir()
	buildPath := buildVigil(t, tmpDir)

	// Run in empty directory
	runCmd := exec.Command(buildPath, "-n")
	runCmd.Dir = tmpDir
	output, _ := runCmd.CombinedOutput()

	if !strings.Contains(string(output), "no CLAUDE.md or AGENTS.md found") {
		t.Errorf("Should error when no CLAUDE.md found")
	}
}
