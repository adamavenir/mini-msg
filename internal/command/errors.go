package command

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func writeCommandError(cmd *cobra.Command, err error) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", err.Error())

	// Check for schema errors and suggest rebuild
	if isSchemaError(err) {
		fmt.Fprintln(cmd.ErrOrStderr(), "Hint: This looks like a schema mismatch. Try: fray rebuild")
	}

	return err
}

// isSchemaError checks if an error is a SQLite schema mismatch.
func isSchemaError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such column") ||
		strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "has no column")
}
