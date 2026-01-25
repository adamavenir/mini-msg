package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func promptChannelName(defaultName string) string {
	if !isTTY(os.Stdin) {
		return defaultName
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Channel name for this project? [%s]: ", defaultName)
	text, _ := reader.ReadString('\n')
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return defaultName
	}
	return trimmed
}

func isTTY(file *os.File) bool {
	stat, _ := file.Stat()
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// promptIssueTracker asks the user to select an issue tracker.
func promptIssueTracker() string {
	if !isTTY(os.Stdin) {
		return ""
	}

	fmt.Println("")
	fmt.Println("Issue tracker? [none, github, jira, linear]")
	fmt.Print("Select [default=none]: ")

	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	trimmed := strings.ToLower(strings.TrimSpace(text))

	if trimmed == "" || trimmed == "none" {
		return ""
	}
	if trimmed == "github" || trimmed == "jira" || trimmed == "linear" {
		return trimmed
	}
	return ""
}

func promptYesNo(question string, defaultYes bool) bool {
	defaultStr := "y/N"
	if defaultYes {
		defaultStr = "Y/n"
	}

	fmt.Printf("%s [%s]: ", question, defaultStr)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	trimmed := strings.ToLower(strings.TrimSpace(text))

	if trimmed == "" {
		return defaultYes
	}
	return trimmed == "y" || trimmed == "yes"
}
