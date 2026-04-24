package upgrade

import (
	"fmt"
	"os"

	"github.com/TD-Yofun/tdu-cli/cmd/utils"
)

func upgradeTdu() error {
	printSection("🔧", "tdu Self-Upgrade")

	utils.PrintStep(1, 2, "📦", "Updating Homebrew tap...")
	updateCmd := newCommand("brew", "update")
	updateCmd.Stdout = os.Stdout
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		printDetail("Warning: brew update failed, continuing with upgrade...")
	}

	utils.PrintStep(2, 2, "⬆️", "Upgrading tdu via Homebrew...")
	cmd := newCommand("brew", "upgrade", "TD-Yofun/tap/tdu")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("brew upgrade failed: %w", err)
	}
	printDetail("tdu upgraded successfully!")
	return nil
}
