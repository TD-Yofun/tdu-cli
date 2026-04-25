package report

import (
	"fmt"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

type reportItem struct {
	Name        string
	Description string
}

var reportItems = []reportItem{
	{Name: "forticlient-vpn", Description: "Report FortiClient VPN issues with logs and system info"},
}

var Cmd = &cobra.Command{
	Use:   "report",
	Short: "Collect diagnostics and report issues to GitHub",
	Long:  "Select a tool to collect diagnostic information and create a GitHub issue.",
	RunE:  runReport,
}

func runReport(cmd *cobra.Command, args []string) error {
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "\U0001F449 {{ .Name | cyan }} - {{ .Description }}",
		Inactive: "  {{ .Name | white }} - {{ .Description }}",
		Selected: "\u2705 {{ .Name | green }}",
	}

	prompt := promptui.Select{
		Label:     "Select a tool to report",
		Items:     reportItems,
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

	selected := reportItems[i]
	fmt.Printf("Reporting %s...\n", selected.Name)

	switch selected.Name {
	case "forticlient-vpn":
		return reportFortiClientVPN()
	default:
		fmt.Println("No report handler for", selected.Name)
	}

	return nil
}
