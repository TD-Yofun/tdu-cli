package upgrade

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/TD-Yofun/tdu-cli/cmd/utils"
)

// Package-level aliases for shared utilities from cmd/utils.
var newCommand = utils.NewCommand
var printSection = utils.PrintSection
var printDetail = utils.PrintDetail
var runSudoCommand = utils.RunSudoCommand
var formatBytes = utils.FormatBytes
var requireGitHubToken = utils.RequireGitHubToken

// printStep wraps utils.PrintStep with a fixed total of 6 steps for upgrade commands.
func printStep(step int, emoji, msg string) {
	utils.PrintStep(step, 6, emoji, msg)
}

// findBinary searches for an executable binary by name in the given directory tree.
func findBinary(dir, name string) (string, error) {
	var found string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == name && info.Mode()&0111 != 0 {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error searching for binary: %w", err)
	}
	if found == "" {
		return "", fmt.Errorf("binary '%s' not found in extracted files", name)
	}
	return found, nil
}
