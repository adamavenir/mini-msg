package command

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed notifier_icon.icns
var notifierIconICNS []byte

const (
	terminalNotifierURL = "https://github.com/julienXX/terminal-notifier/releases/download/2.0.0/terminal-notifier-2.0.0.zip"
	notifierBundleID    = "com.fray.notifier"
	notifierAppName     = "Fray-Notifier.app"
)

// focusFrayScript is a shell script that focuses the terminal window running fray.
// Uses Window menu to find windows with "fray" in the title, which works reliably
// across different terminal contexts (including when already in a terminal).
// Accepts an optional target argument (thread#message) which gets written to /tmp/fray-goto
// for fray chat to pick up and navigate to.
const focusFrayScript = `#!/bin/bash
TARGET="$1"

osascript << 'APPLESCRIPT'
-- Try each terminal app (check via System Events to avoid "where is app?" prompts)
set terminals to {"Ghostty", "iTerm2", "Terminal"}
tell application "System Events"
    set runningApps to name of every process
end tell

repeat with termApp in terminals
    if termApp is in runningApps then
        try
            -- Activate Finder briefly for context switch
            tell application "Finder" to activate
            delay 0.2

            -- Activate terminal and wait for it to be frontmost
            tell application termApp to activate
            delay 0.3

            tell application "System Events"
                -- Wait for terminal to be frontmost
                repeat 10 times
                    if name of first process whose frontmost is true is termApp then
                        exit repeat
                    end if
                    delay 0.1
                end repeat

                -- Click the fray window in terminal's menu
                tell process termApp
                    set windowMenu to menu "Window" of menu bar 1
                    repeat with menuItem in menu items of windowMenu
                        set itemName to name of menuItem
                        if itemName starts with "fray" then
                            click menuItem
                            exit repeat
                        end if
                    end repeat
                end tell
            end tell
        end try
    end if
end repeat
APPLESCRIPT

# Write navigation target for fray chat to pick up
if [ -n "$TARGET" ]; then
    echo "$TARGET" > /tmp/fray-goto
fi
`

// NewInstallNotifierCmd creates the install-notifier command.
func NewInstallNotifierCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "install-notifier",
		Short: "Install Fray-Notifier.app for macOS notifications with custom icon",
		Long: `Downloads and configures a custom notification app for macOS.

This creates ~/Applications/Fray-Notifier.app which displays the fray icon
in notification center instead of the generic terminal icon.

Only needed on macOS. Other platforms use system notifications automatically.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "darwin" {
				fmt.Fprintln(cmd.OutOrStdout(), "This command is only needed on macOS.")
				fmt.Fprintln(cmd.OutOrStdout(), "Other platforms use system notifications automatically.")
				return nil
			}

			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home dir: %w", err)
			}

			appsDir := filepath.Join(home, "Applications")
			appPath := filepath.Join(appsDir, notifierAppName)

			// Check if already installed
			if _, err := os.Stat(appPath); err == nil && !force {
				fmt.Fprintln(cmd.OutOrStdout(), "Fray-Notifier.app is already installed.")
				fmt.Fprintln(cmd.OutOrStdout(), "Use --force to reinstall.")
				return nil
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Downloading terminal-notifier...")

			// Download terminal-notifier
			resp, err := http.Get(terminalNotifierURL)
			if err != nil {
				return fmt.Errorf("download terminal-notifier: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("download failed: %s", resp.Status)
			}

			zipData, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read download: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Extracting and customizing...")

			// Extract zip
			zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
			if err != nil {
				return fmt.Errorf("open zip: %w", err)
			}

			// Create temp directory for extraction
			tmpDir, err := os.MkdirTemp("", "fray-notifier-")
			if err != nil {
				return fmt.Errorf("create temp dir: %w", err)
			}
			defer os.RemoveAll(tmpDir)

			// Extract all files
			for _, f := range zipReader.File {
				destPath := filepath.Join(tmpDir, f.Name)

				if f.FileInfo().IsDir() {
					os.MkdirAll(destPath, f.Mode())
					continue
				}

				if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
					return fmt.Errorf("create dir: %w", err)
				}

				destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
				if err != nil {
					return fmt.Errorf("create file: %w", err)
				}

				srcFile, err := f.Open()
				if err != nil {
					destFile.Close()
					return fmt.Errorf("open zip file: %w", err)
				}

				_, err = io.Copy(destFile, srcFile)
				srcFile.Close()
				destFile.Close()
				if err != nil {
					return fmt.Errorf("extract file: %w", err)
				}
			}

			// Find the extracted app
			extractedApp := filepath.Join(tmpDir, "terminal-notifier.app")
			if _, err := os.Stat(extractedApp); err != nil {
				return fmt.Errorf("terminal-notifier.app not found in archive")
			}

			// Replace icon
			iconPath := filepath.Join(extractedApp, "Contents", "Resources", "Terminal.icns")
			if err := os.WriteFile(iconPath, notifierIconICNS, 0644); err != nil {
				return fmt.Errorf("write icon: %w", err)
			}

			// Update bundle ID in Info.plist
			plistPath := filepath.Join(extractedApp, "Contents", "Info.plist")
			plistData, err := os.ReadFile(plistPath)
			if err != nil {
				return fmt.Errorf("read plist: %w", err)
			}
			plistData = []byte(strings.Replace(string(plistData), "fr.julienxx.oss.terminal-notifier", notifierBundleID, -1))
			if err := os.WriteFile(plistPath, plistData, 0644); err != nil {
				return fmt.Errorf("write plist: %w", err)
			}

			// Write focus script to MacOS directory
			focusScriptPath := filepath.Join(extractedApp, "Contents", "MacOS", "focus-fray")
			if err := os.WriteFile(focusScriptPath, []byte(focusFrayScript), 0755); err != nil {
				return fmt.Errorf("write focus script: %w", err)
			}

			// Ensure ~/Applications exists
			if err := os.MkdirAll(appsDir, 0755); err != nil {
				return fmt.Errorf("create Applications dir: %w", err)
			}

			// Remove existing if force
			if force {
				os.RemoveAll(appPath)
			}

			// Move to ~/Applications
			if err := os.Rename(extractedApp, appPath); err != nil {
				return fmt.Errorf("install app: %w", err)
			}

			// Register with Launch Services
			_ = exec.Command("/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister", "-f", appPath).Run()

			fmt.Fprintln(cmd.OutOrStdout(), "✓ Installed Fray-Notifier.app to ~/Applications/")
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintln(cmd.OutOrStdout(), "Notifications in fray chat will now show the fray icon.")

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Reinstall even if already installed")

	return cmd
}
