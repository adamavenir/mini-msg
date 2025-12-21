package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeCommand(cmd *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return buf.String(), err
}

func TestRootCommandVersion(t *testing.T) {
	cmd := NewRootCmd("test")

	output, err := executeCommand(cmd, "--version")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(output, "mm version test") {
		t.Fatalf("expected version output, got %q", output)
	}
}

func TestRootCommandHelp(t *testing.T) {
	cmd := NewRootCmd("test")

	output, err := executeCommand(cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(output, "Mini Messenger") {
		t.Fatalf("expected help output, got %q", output)
	}
}
