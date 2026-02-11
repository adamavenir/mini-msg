package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/hostedsync"
	"github.com/spf13/cobra"
)

type syncStatusResult struct {
	Configured       bool   `json:"configured"`
	Backend          string `json:"backend,omitempty"`
	Path             string `json:"path,omitempty"`
	HostedURL        string `json:"hosted_url,omitempty"`
	HostedMachineID  string `json:"hosted_machine_id,omitempty"`
	HostedRegistered bool   `json:"hosted_registered,omitempty"`
	SharedPath       string `json:"shared_path,omitempty"`
	SharedTarget     string `json:"shared_target,omitempty"`
	IsSymlink        bool   `json:"is_symlink"`
}

// NewSyncCmd creates the sync command.
func NewSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Manage sync backends",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		NewSyncStatusCmd(),
		NewSyncSetupCmd(),
		NewSyncDaemonCmd(),
	)

	return cmd
}

// NewSyncStatusCmd reports current sync configuration.
func NewSyncStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show sync configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			config := ctx.ProjectConfig
			var backend string
			var path string
			var hostedURL string
			var hostedMachineID string
			var hostedRegistered bool
			configured := false
			if config != nil && config.Sync != nil {
				configured = true
				backend = config.Sync.Backend
				path = config.Sync.Path
				hostedURL = config.Sync.HostedURL
			}

			if auth, err := hostedsync.LoadAuth(ctx.Project.Root); err != nil {
				return writeCommandError(cmd, err)
			} else if auth != nil {
				if auth.HostedURL != "" {
					hostedURL = auth.HostedURL
				}
				hostedMachineID = auth.MachineID
				hostedRegistered = auth.Token != ""
			}

			sharedPath := filepath.Join(ctx.Project.Root, ".fray", "shared")
			var sharedTarget string
			isSymlink := false
			if info, err := os.Lstat(sharedPath); err == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					isSymlink = true
					if target, err := os.Readlink(sharedPath); err == nil {
						sharedTarget = target
					}
				}
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(syncStatusResult{
					Configured:       configured,
					Backend:          backend,
					Path:             path,
					HostedURL:        hostedURL,
					HostedMachineID:  hostedMachineID,
					HostedRegistered: hostedRegistered,
					SharedPath:       sharedPath,
					SharedTarget:     sharedTarget,
					IsSymlink:        isSymlink,
				})
			}

			if !configured {
				fmt.Fprintln(cmd.OutOrStdout(), "Sync: not configured")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Backend: %s\n", backend)
				fmt.Fprintf(cmd.OutOrStdout(), "Path:    %s\n", path)
			}
			if hostedURL != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Hosted:  %s\n", hostedURL)
				if hostedMachineID != "" {
					status := "not registered"
					if hostedRegistered {
						status = "registered"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "Machine: %s (%s)\n", hostedMachineID, status)
				}
			}

			if isSymlink {
				fmt.Fprintf(cmd.OutOrStdout(), "Shared:  %s -> %s\n", sharedPath, sharedTarget)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Shared:  %s\n", sharedPath)
			}
			return nil
		},
	}

	return cmd
}

// NewSyncSetupCmd configures a sync backend and creates shared symlink.
func NewSyncSetupCmd() *cobra.Command {
	var useICloud bool
	var useDropbox bool
	var customPath string
	var hostedURL string
	var skipRegister bool

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure sync backend",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			target, err := resolveSyncSetupTarget(useICloud, useDropbox, customPath, hostedURL)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if target.backend == "hosted" {
				if !db.IsMultiMachineMode(ctx.Project.DBPath) {
					return writeCommandError(cmd, fmt.Errorf("hosted sync requires multi-machine storage (run `fray migrate --multi-machine`)"))
				}
				channelID := ctx.ChannelID
				if channelID == "" && ctx.ProjectConfig != nil {
					channelID = ctx.ProjectConfig.ChannelID
				}
				if channelID == "" {
					return writeCommandError(cmd, fmt.Errorf("channel_id missing; run `fray init` first"))
				}
				machineID := db.GetLocalMachineID(ctx.Project.Root)
				if machineID == "" {
					return writeCommandError(cmd, fmt.Errorf("local machine id missing; run `fray init` first"))
				}
				sharedPath := filepath.Join(ctx.Project.Root, ".fray", "shared")
				if err := ensureSharedDir(sharedPath, machineID); err != nil {
					return writeCommandError(cmd, err)
				}

				syncConfig := &db.ProjectSyncConfig{
					Backend:   "hosted",
					Path:      sharedPath,
					HostedURL: target.hostedURL,
				}
				if _, err := db.UpdateProjectConfig(ctx.Project.DBPath, db.ProjectConfig{Sync: syncConfig}); err != nil {
					return writeCommandError(cmd, err)
				}

				var hostedMachineID string
				registered := false
				if !skipRegister {
					client, err := hostedsync.NewClient(target.hostedURL, "")
					if err != nil {
						return writeCommandError(cmd, err)
					}
					deviceInfo := buildDeviceInfo()
					resp, err := client.RegisterMachine(cmd.Context(), hostedsync.RegisterMachineRequest{
						ChannelID:  channelID,
						MachineID:  machineID,
						DeviceInfo: deviceInfo,
					})
					if err != nil {
						return writeCommandError(cmd, err)
					}
					registered = resp.Token != ""
					hostedMachineID = resp.MachineID
					auth := hostedsync.Auth{
						ChannelID:    channelID,
						MachineID:    machineID,
						HostedURL:    target.hostedURL,
						Token:        resp.Token,
						RegisteredAt: time.Now().Unix(),
					}
					if err := hostedsync.SaveAuth(ctx.Project.Root, auth); err != nil {
						return writeCommandError(cmd, err)
					}
				}

				if err := hostedsync.SaveState(ctx.Project.Root, &hostedsync.State{
					ChannelID: channelID,
					Streams:   map[string]hostedsync.StreamCursor{},
				}); err != nil {
					return writeCommandError(cmd, err)
				}

				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(syncStatusResult{
						Configured:       true,
						Backend:          "hosted",
						Path:             sharedPath,
						HostedURL:        target.hostedURL,
						HostedMachineID:  hostedMachineID,
						HostedRegistered: registered,
						SharedPath:       sharedPath,
						IsSymlink:        false,
					})
				}

				fmt.Fprintf(cmd.OutOrStdout(), "✓ Hosted sync configured for %s\n", target.hostedURL)
				if registered {
					fmt.Fprintf(cmd.OutOrStdout(), "✓ Registered machine %s\n", machineID)
				} else if skipRegister {
					fmt.Fprintln(cmd.OutOrStdout(), "• Machine registration skipped")
				}
				return nil
			}

			channelName := channelNameForSync(ctx.Project.Root, ctx.ProjectConfig)
			targetPath := filepath.Join(target.basePath, channelName, "shared")
			if err := ensureSharedSymlink(ctx.Project.Root, targetPath, ctx.Force); err != nil {
				return writeCommandError(cmd, err)
			}

			syncConfig := &db.ProjectSyncConfig{
				Backend: target.backend,
				Path:    targetPath,
			}
			if _, err := db.UpdateProjectConfig(ctx.Project.DBPath, db.ProjectConfig{Sync: syncConfig}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(syncStatusResult{
					Configured:   true,
					Backend:      target.backend,
					Path:         targetPath,
					SharedPath:   filepath.Join(ctx.Project.Root, ".fray", "shared"),
					SharedTarget: targetPath,
					IsSymlink:    true,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✓ Linked .fray/shared to %s\n", targetPath)
			return nil
		},
	}

	cmd.Flags().BoolVar(&useICloud, "icloud", false, "use iCloud Drive for sync")
	cmd.Flags().BoolVar(&useDropbox, "dropbox", false, "use Dropbox for sync")
	cmd.Flags().StringVar(&customPath, "path", "", "custom sync base path")
	cmd.Flags().StringVar(&hostedURL, "hosted", "", "use hosted sync backend (URL)")
	cmd.Flags().BoolVar(&skipRegister, "no-register", false, "skip hosted machine registration")

	return cmd
}

type syncSetupTarget struct {
	backend   string
	basePath  string
	hostedURL string
}

func resolveSyncSetupTarget(useICloud, useDropbox bool, customPath string, hostedURL string) (syncSetupTarget, error) {
	count := 0
	if useICloud {
		count++
	}
	if useDropbox {
		count++
	}
	if customPath != "" {
		count++
	}
	if hostedURL != "" {
		count++
	}
	if count == 0 {
		return syncSetupTarget{}, fmt.Errorf("choose one of --icloud, --dropbox, --path, or --hosted")
	}
	if count > 1 {
		return syncSetupTarget{}, fmt.Errorf("choose only one sync backend")
	}

	switch {
	case useICloud:
		home, err := os.UserHomeDir()
		if err != nil {
			return syncSetupTarget{}, err
		}
		return syncSetupTarget{backend: "icloud", basePath: filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs", "fray-sync")}, nil
	case useDropbox:
		home, err := os.UserHomeDir()
		if err != nil {
			return syncSetupTarget{}, err
		}
		return syncSetupTarget{backend: "dropbox", basePath: filepath.Join(home, "Dropbox", "fray-sync")}, nil
	case hostedURL != "":
		normalized, err := hostedsync.NormalizeBaseURL(hostedURL)
		if err != nil {
			return syncSetupTarget{}, err
		}
		return syncSetupTarget{backend: "hosted", hostedURL: normalized}, nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return syncSetupTarget{}, err
		}
		resolved, err := expandUserPath(customPath, home)
		if err != nil {
			return syncSetupTarget{}, err
		}
		return syncSetupTarget{backend: "path", basePath: resolved}, nil
	}
}

func channelNameForSync(projectRoot string, config *db.ProjectConfig) string {
	if config != nil && config.ChannelName != "" {
		return config.ChannelName
	}
	return filepath.Base(projectRoot)
}

func expandUserPath(path, home string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	if path == "~" {
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func ensureSharedSymlink(projectRoot, targetPath string, force bool) error {
	sharedPath := filepath.Join(projectRoot, ".fray", "shared")
	if filepath.Clean(sharedPath) == filepath.Clean(targetPath) {
		return nil
	}

	info, err := os.Lstat(sharedPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			current, err := os.Readlink(sharedPath)
			if err != nil {
				return err
			}
			if filepath.Clean(current) == filepath.Clean(targetPath) {
				return nil
			}
			if !force {
				return fmt.Errorf("shared is already linked to %s (use --force to replace)", current)
			}
			if err := os.Remove(sharedPath); err != nil {
				return err
			}
		} else if info.IsDir() {
			if err := moveSharedDir(sharedPath, targetPath, force); err != nil {
				return err
			}
		} else {
			if !force {
				return fmt.Errorf("shared path exists and is not a directory")
			}
			if err := os.Remove(sharedPath); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(sharedPath), 0o755); err != nil {
		return err
	}
	return os.Symlink(targetPath, sharedPath)
}

func moveSharedDir(sharedPath, targetPath string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	if info, err := os.Stat(targetPath); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("sync target exists and is not a directory: %s", targetPath)
		}
		empty, err := dirIsEmpty(targetPath)
		if err != nil {
			return err
		}
		if !empty && !force {
			return fmt.Errorf("sync target already exists: %s", targetPath)
		}
		if err := os.RemoveAll(targetPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return os.Rename(sharedPath, targetPath)
}

func dirIsEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func ensureSharedDir(sharedPath, machineID string) error {
	if sharedPath == "" {
		return fmt.Errorf("shared path required")
	}
	if err := os.MkdirAll(sharedPath, 0o755); err != nil {
		return err
	}
	machinesPath := filepath.Join(sharedPath, "machines")
	if err := os.MkdirAll(machinesPath, 0o755); err != nil {
		return err
	}
	if machineID != "" {
		if err := os.MkdirAll(filepath.Join(machinesPath, machineID), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func buildDeviceInfo() map[string]string {
	info := map[string]string{
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"fray_version": Version,
	}
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		info["hostname"] = hostname
	}
	return info
}
