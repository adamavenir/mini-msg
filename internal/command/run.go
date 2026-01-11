package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	mlld "github.com/mlld-lang/mlld/sdk/go"
	"github.com/spf13/cobra"
)

func NewRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [script-name]",
		Short: "Run mlld scripts from .fray/llm/run/",
		Long: `Run mlld scripts from the project's .fray/llm/run/ directory.

Without arguments, lists available scripts.
With a script name, executes that script.

Scripts can use @proj/ to reference files in the project root,
relative to the .fray directory.`,
		Example: `  fray run                # List available scripts
  fray run hello          # Run .fray/llm/run/hello.mld
  fray run build --debug  # Run with debug output`,
		RunE: func(cmd *cobra.Command, args []string) error {
			debug, _ := cmd.Flags().GetBool("debug")
			timeout, _ := cmd.Flags().GetDuration("timeout")

			project, err := core.DiscoverProject("")
			if err != nil {
				return fmt.Errorf("not in a fray project (run fray init first)")
			}

			runDir := filepath.Join(project.Root, ".fray", "llm", "run")
			if _, err := os.Stat(runDir); os.IsNotExist(err) {
				return fmt.Errorf("no scripts directory (create .fray/llm/run/*.mld)")
			}

			scripts, err := listScripts(runDir)
			if err != nil {
				return err
			}

			if len(args) == 0 {
				return listScriptsCmd(cmd, scripts)
			}

			scriptName := args[0]
			scriptPath := filepath.Join(runDir, scriptName+".mld")
			if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
				return fmt.Errorf("script not found: %s\n\nAvailable scripts:\n%s",
					scriptName, formatScriptList(scripts))
			}

			return runScript(cmd, scriptPath, scriptName, timeout, debug)
		},
	}

	cmd.Flags().Bool("debug", false, "show execution metrics")
	cmd.Flags().Duration("timeout", 5*time.Minute, "script timeout")

	return cmd
}

func listScripts(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var scripts []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".mld") {
			scripts = append(scripts, strings.TrimSuffix(name, ".mld"))
		}
	}
	return scripts, nil
}

func listScriptsCmd(cmd *cobra.Command, scripts []string) error {
	if len(scripts) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No scripts found in .fray/llm/run/")
		fmt.Fprintln(cmd.OutOrStdout(), "")
		fmt.Fprintln(cmd.OutOrStdout(), "Create a .mld file to get started:")
		fmt.Fprintln(cmd.OutOrStdout(), "  .fray/llm/run/hello.mld")
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Available scripts:")
	for _, name := range scripts {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", name)
	}
	return nil
}

func formatScriptList(scripts []string) string {
	if len(scripts) == 0 {
		return "  (none)"
	}
	var lines []string
	for _, s := range scripts {
		lines = append(lines, "  "+s)
	}
	return strings.Join(lines, "\n")
}

func runScript(cmd *cobra.Command, scriptPath, scriptName string, timeout time.Duration, debug bool) error {
	project, _ := core.DiscoverProject("")
	frayDir := filepath.Join(project.Root, ".fray")

	client := mlld.New()
	client.Timeout = timeout
	client.WorkingDir = frayDir

	result, err := client.Execute(scriptPath, nil, nil)
	if err != nil {
		return fmt.Errorf("script error: %v", err)
	}

	output := strings.TrimSpace(result.Output)
	if output != "" {
		fmt.Fprintln(cmd.OutOrStdout(), output)
	}

	if debug && result.Metrics != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "\n[%s] %.0fms total (parse: %.0fms, eval: %.0fms)\n",
			scriptName,
			result.Metrics.TotalMs,
			result.Metrics.ParseMs,
			result.Metrics.EvaluateMs,
		)
	}

	return nil
}
