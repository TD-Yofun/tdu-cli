package fix

import (
	"fmt"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

type fixItem struct {
	Name        string
	Description string
}

var fixItems = []fixItem{
	{Name: "forticlient-vpn", Description: "Fix FortiClient VPN blank screen and SAML login issues"},
}

var Cmd = &cobra.Command{
	Use:   "fix",
	Short: "Fix known issues with tools interactively",
	Long:  "Select a tool to apply known fixes from an interactive list.",
	RunE:  runFix,
}

func runFix(cmd *cobra.Command, args []string) error {
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "\U0001F449 {{ .Name | cyan }} - {{ .Description }}",
		Inactive: "  {{ .Name | white }} - {{ .Description }}",
		Selected: "\u2705 {{ .Name | green }}",
	}

	prompt := promptui.Select{
		Label:     "Select a tool to fix",
		Items:     fixItems,
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

	selected := fixItems[i]
	fmt.Printf("Fixing %s...\n", selected.Name)

	switch selected.Name {
	case "forticlient-vpn":
		return fixFortiClientVPN()
	default:
		fmt.Println("No fix handler for", selected.Name)
	}

	return nil
}
