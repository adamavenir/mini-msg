package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func writeCommandError(cmd *cobra.Command, err error) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", err.Error())
	return err
}
