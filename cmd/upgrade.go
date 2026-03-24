package cmd

import (
	"fmt"
	"os"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

type upgradeItem struct {
	Name        string
	Description string
}

var upgradeItems = []upgradeItem{
	{Name: "tdu", Description: "Upgrade tdu CLI itself to the latest version"},
	// Add more upgrade targets here as needed
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade tools interactively",
	Long:  "Select a tool to upgrade from an interactive list.",
	RunE:  runUpgrade,
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "\U0001F449 {{ .Name | cyan }} - {{ .Description }}",
		Inactive: "  {{ .Name | white }} - {{ .Description }}",
		Selected: "\u2705 {{ .Name | green }}",
	}

	prompt := promptui.Select{
		Label:     "Select a tool to upgrade",
		Items:     upgradeItems,
		Templates: templates,
		Size:      10,
	}

	i, _, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrInterrupt {
			fmt.Println("Cancelled.")
			return nil
		}
		return fmt.Errorf("prompt failed: %w", err)
	}

	selected := upgradeItems[i]
	fmt.Printf("Upgrading %s...\n", selected.Name)

	switch selected.Name {
	case "tdu":
		return upgradeTdu()
	default:
		fmt.Println("No upgrade handler for", selected.Name)
	}

	return nil
}

func upgradeTdu() error {
	fmt.Println("Upgrading tdu via Homebrew...")
	// Use os/exec to run brew upgrade
	cmd := newCommand("brew", "upgrade", "TD-Yofun/tap/tdu")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("brew upgrade failed: %w", err)
	}
	fmt.Println("tdu upgraded successfully!")
	return nil
}
