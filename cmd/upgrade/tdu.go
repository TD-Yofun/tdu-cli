package upgrade

import (
	"fmt"
	"os"
)

func upgradeTdu() error {
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
