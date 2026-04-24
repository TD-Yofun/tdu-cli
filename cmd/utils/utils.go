package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// NewCommand creates an exec.Cmd. Extracted for testability.
func NewCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// PrintSection prints a visually prominent section header with emoji borders.
func PrintSection(emoji, title string) {
	fmt.Println()
	fmt.Printf("  %s ═══════════════════════════════════════════\n", emoji)
	fmt.Printf("  %s  %s\n", emoji, title)
	fmt.Printf("  %s ═══════════════════════════════════════════\n", emoji)
	fmt.Println()
}

// PrintStep prints a numbered step indicator with emoji.
func PrintStep(step, total int, emoji, msg string) {
	fmt.Printf("  [%d/%d] %s %s\n", step, total, emoji, msg)
}

// PrintDetail prints an indented detail line.
func PrintDetail(msg string) {
	fmt.Printf("        %s\n", msg)
}

// RunSudoCommand runs a command with sudo and shows a prominent error on permission failure.
func RunSudoCommand(description string, args ...string) error {
	sudoArgs := append([]string{}, args...)
	cmd := NewCommand("sudo", sudoArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if ok := errors.As(err, &exitErr); ok && exitErr.ExitCode() == 1 {
			fmt.Println()
			fmt.Println("  ❌ ═══════════════════════════════════════════")
			fmt.Println("  ❌  Permission Denied")
			fmt.Println("  ❌ ═══════════════════════════════════════════")
			fmt.Println()
			fmt.Println("  The current user does not have sudo privileges.")
			fmt.Println("  To fix this, try one of the following:")
			fmt.Println()
			fmt.Println("    1. Run with a user that has sudo access")
			fmt.Println("    2. Ask your administrator to add you to the sudoers file:")
			fmt.Println("       sudo visudo  →  add: <your_username> ALL=(ALL) ALL")
			fmt.Println()
		}
		return fmt.Errorf("%s failed: %w", description, err)
	}
	return nil
}

// FormatBytes formats a byte count into a human-readable string (e.g. "1.5 MB").
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// RequireGitHubToken checks for the HOMEBREW_GITHUB_API_TOKEN environment variable.
// Returns the token if set, or prints a prominent error message and returns an error.
func RequireGitHubToken() (string, error) {
	token := os.Getenv("HOMEBREW_GITHUB_API_TOKEN")
	if token == "" {
		fmt.Println()
		fmt.Println("  ❌ ═══════════════════════════════════════════")
		fmt.Println("  ❌  HOMEBREW_GITHUB_API_TOKEN is not set")
		fmt.Println("  ❌ ═══════════════════════════════════════════")
		fmt.Println()
		fmt.Println("  Please add the following to your shell profile (~/.zshrc or ~/.bashrc):")
		fmt.Println()
		fmt.Println("    export HOMEBREW_GITHUB_API_TOKEN=your_github_personal_access_token")
		fmt.Println()
		fmt.Println("  Then restart your terminal or run: source ~/.zshrc")
		return "", fmt.Errorf("HOMEBREW_GITHUB_API_TOKEN is not set")
	}
	return token, nil
}
