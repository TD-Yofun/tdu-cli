package upgrade

import (
	"fmt"
	"os"
)

func upgradeTdu() error {
	fmt.Println("Updating Homebrew tap...")
	updateCmd := newCommand("brew", "update")
	updateCmd.Stdout = os.Stdout
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		fmt.Println("Warning: brew update failed, continuing with upgrade...")
	}

	fmt.Println("Upgrading tdu via Homebrew...")
	cmd := newCommand("brew", "upgrade", "TD-Yofun/tap/tdu")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("brew upgrade failed: %w", err)
	}
	fmt.Println("tdu upgraded successfully!")
	return nil
}
