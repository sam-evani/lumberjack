package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func executeRootForTest(args ...string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	rootCmd.SetArgs(args)
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	err := rootCmd.Execute()

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)

	return stdout.String(), stderr.String(), err
}

func TestRootCommandIsSilentOnCommandErrors(t *testing.T) {
	_, stderr, err := executeRootForTest("unknown")
	if err == nil {
		t.Fatal("expected unknown command to return an error")
	}
	if stderr != "" {
		t.Fatalf("expected Cobra to leave command error printing to Execute, got stderr %q", stderr)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	stdout, stderr, err := executeRootForTest("version", "extra")
	if err == nil {
		t.Fatal("expected version with extra arg to return an error")
	}
	if stdout != "" {
		t.Fatalf("expected no version output, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected Cobra to leave argument error printing to Execute, got stderr %q", stderr)
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Fatalf("expected error to reject extra arg, got %v", err)
	}
}

func TestHelpIncludesListCommand(t *testing.T) {
	stdout, stderr, err := executeRootForTest("help")
	if err != nil {
		t.Fatalf("expected help to succeed, got %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "list") || !strings.Contains(stdout, "Print worktrees as TSV") {
		t.Fatalf("expected help to include list command, got %q", stdout)
	}
}
