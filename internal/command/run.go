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
	var inject []string

	cmd := &cobra.Command{
		Use:   "run [script-name] [@agent] [message]",
		Short: "Run mlld scripts from .fray/llm/",
		Long: `Run mlld scripts from the project's .fray/llm/ directory.

Searches for scripts in: llm/run/, llm/slash/, llm/prompts/

Without arguments, lists available scripts.
With a script name, executes that script.

Scripts can use @proj/ to reference files in the project root,
relative to the .fray directory.

Arguments after the script name are parsed as payload:
- @agent arguments become agent=<name> in payload
- Remaining text becomes message=<text> in payload
- Use --inject for explicit key=value pairs`,
		Example: `  fray run                        # List available scripts
  fray run hello                  # Run .fray/llm/run/hello.mld
  fray run fly @opus              # Run fly.mld with agent=opus
  fray run fly @opus "check in"   # Run with agent + message
  fray run build --inject env=prod  # Explicit injection`,
		RunE: func(cmd *cobra.Command, args []string) error {
			debug, _ := cmd.Flags().GetBool("debug")
			timeout, _ := cmd.Flags().GetDuration("timeout")

			project, err := core.DiscoverProject("")
			if err != nil {
				return fmt.Errorf("not in a fray project (run fray init first)")
			}

			llmDir := filepath.Join(project.Root, ".fray", "llm")
			runDir := filepath.Join(llmDir, "run")
			slashDir := filepath.Join(llmDir, "slash")
			promptsDir := filepath.Join(llmDir, "prompts")

			allScripts := make(map[string]string)
			for _, dir := range []string{runDir, slashDir, promptsDir} {
				if scripts, err := listScriptsWithDir(dir); err == nil {
					for name, path := range scripts {
						if _, exists := allScripts[name]; !exists {
							allScripts[name] = path
						}
					}
				}
			}

			if len(allScripts) == 0 {
				return fmt.Errorf("no scripts found (create .fray/llm/run/*.mld or .fray/llm/slash/*.mld)")
			}

			if len(args) == 0 {
				return listScriptsMapCmd(cmd, allScripts)
			}

			scriptName := args[0]
			scriptPath, exists := allScripts[scriptName]
			if !exists {
				return fmt.Errorf("script not found: %s\n\nAvailable scripts:\n%s",
					scriptName, formatScriptMap(allScripts))
			}

			payload := buildPayload(args[1:], inject)
			return runScriptWithPayload(cmd, scriptPath, scriptName, timeout, debug, payload)
		},
	}

	cmd.Flags().Bool("debug", false, "show execution metrics")
	cmd.Flags().Duration("timeout", 5*time.Minute, "script timeout")
	cmd.Flags().StringArrayVar(&inject, "inject", nil, "inject key=value pairs into payload")

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
	return runScriptWithPayload(cmd, scriptPath, scriptName, timeout, debug, nil)
}

func runScriptWithPayload(cmd *cobra.Command, scriptPath, scriptName string, timeout time.Duration, debug bool, payload map[string]any) error {
	project, _ := core.DiscoverProject("")
	frayDir := filepath.Join(project.Root, ".fray")

	client := mlld.New()
	client.Timeout = timeout
	client.WorkingDir = frayDir

	result, err := client.Execute(scriptPath, payload, nil)
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

func listScriptsWithDir(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	scripts := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".mld") {
			scriptName := strings.TrimSuffix(name, ".mld")
			scripts[scriptName] = filepath.Join(dir, name)
		}
	}
	return scripts, nil
}

func listScriptsMapCmd(cmd *cobra.Command, scripts map[string]string) error {
	if len(scripts) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No scripts found in .fray/llm/")
		fmt.Fprintln(cmd.OutOrStdout(), "")
		fmt.Fprintln(cmd.OutOrStdout(), "Create a .mld file to get started:")
		fmt.Fprintln(cmd.OutOrStdout(), "  .fray/llm/run/hello.mld")
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Available scripts:")
	for name := range scripts {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", name)
	}
	return nil
}

func formatScriptMap(scripts map[string]string) string {
	if len(scripts) == 0 {
		return "  (none)"
	}
	var lines []string
	for name := range scripts {
		lines = append(lines, "  "+name)
	}
	return strings.Join(lines, "\n")
}

func buildPayload(args []string, inject []string) map[string]any {
	payload := make(map[string]any)

	var messageParts []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "@") {
			payload["agent"] = strings.TrimPrefix(arg, "@")
		} else {
			messageParts = append(messageParts, arg)
		}
	}

	if len(messageParts) > 0 {
		payload["message"] = strings.Join(messageParts, " ")
	}

	for _, kv := range inject {
		if idx := strings.Index(kv, "="); idx > 0 {
			key := kv[:idx]
			value := kv[idx+1:]
			payload[key] = value
		}
	}

	if len(payload) == 0 {
		return nil
	}
	return payload
}
