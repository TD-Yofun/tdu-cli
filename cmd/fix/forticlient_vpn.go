package fix

import (
	"fmt"
	"os"
	"strings"

	"github.com/TD-Yofun/tdu-cli/cmd/utils"
	"github.com/manifoldco/promptui"
)

const (
	fortiClientApp          = "/Applications/FortiClient.app"
	servctl2Plist           = "/Library/LaunchDaemons/com.fortinet.forticlient.servctl2.plist"
	servctl2ServiceLabel    = "com.fortinet.fctservctl2"
	privilegedHelperPlist   = "/Library/LaunchDaemons/com.fortinet.forticlient.macos.PrivilegedHelper.plist"
	fortiTrayApp            = "/Applications/FortiClient.app/Contents/Resources/runtime.helper/FortiClientAgent.app/Contents/Resources/FortiTray/FortiTray.app"
	fortiClientProcesses    = "FortiClient FortiClientAgent FctMiscAgent FortiTray"
	fortiClientVersionPlist = "/Applications/FortiClient.app/Contents/Info.plist"
)

// Package-level aliases for shared utilities.
var newCommand = utils.NewCommand
var printSection = utils.PrintSection
var printStep = utils.PrintStep
var printDetail = utils.PrintDetail
var runSudoCommand = utils.RunSudoCommand

func fixFortiClientVPN() error {
	printSection("🔧", "FortiClient VPN Fix")

	// Check if FortiClient is installed
	if _, err := os.Stat(fortiClientApp); os.IsNotExist(err) {
		printDetail("❌ FortiClient VPN is not installed at " + fortiClientApp)
		return fmt.Errorf("FortiClient VPN is not installed")
	}

	// Show current version
	version := getFortiClientVersion()
	if version != "" {
		printDetail(fmt.Sprintf("FortiClient version: %s", version))
	}
	printDetail(fmt.Sprintf("App path: %s", fortiClientApp))

	totalSteps := 6

	// ═══════════════ Step 1: Diagnose ═══════════════
	printStep(1, totalSteps, "🔍", "Diagnosing FortiClient VPN status...")

	servctl2Running := isProcessRunning("fctservctl2")
	privilegedHelperRunning := isProcessRunning("PrivilegedHelper")
	fortiTrayRunning := isProcessRunning("FortiTray")

	printDetail(fmt.Sprintf("fctservctl2 (core daemon):  %s", statusEmoji(servctl2Running)))
	printDetail(fmt.Sprintf("PrivilegedHelper:           %s", statusEmoji(privilegedHelperRunning)))
	printDetail(fmt.Sprintf("FortiTray (SAML handler):   %s", statusEmoji(fortiTrayRunning)))

	if servctl2Running && privilegedHelperRunning && fortiTrayRunning {
		fmt.Println()
		fmt.Println("  ✅ All FortiClient VPN services are running normally!")
		fmt.Println()
		return nil
	}

	// Show what will be fixed
	fmt.Println()
	printDetail("Issues detected:")
	if !servctl2Running {
		printDetail("  • fctservctl2 daemon is not running (causes blank screen)")
	}
	if !privilegedHelperRunning {
		printDetail("  • PrivilegedHelper is not running (causes blank screen)")
	}
	if !fortiTrayRunning {
		printDetail("  • FortiTray is not running (causes SAML login failure)")
	}

	// ═══════════════ Step 2: Confirm with user ═══════════════
	printStep(2, totalSteps, "❓", "Requesting authorization...")

	fmt.Println()
	printDetail("The following actions require sudo (administrator) privileges:")
	if !servctl2Running {
		printDetail("  • Load and start fctservctl2 daemon")
	}
	if !privilegedHelperRunning {
		printDetail("  • Load PrivilegedHelper daemon")
	}
	printDetail("  • Restart FortiClient processes")
	fmt.Println()

	confirmPrompt := promptui.Prompt{
		Label:     "  Do you want to proceed with the fix",
		IsConfirm: true,
	}
	_, err := confirmPrompt.Run()
	if err != nil {
		fmt.Println()
		fmt.Println("  ❌ Operation cancelled by user.")
		fmt.Println()
		return nil
	}
	fmt.Println()

	// ═══════════════ Step 3: Fix daemons ═══════════════
	printStep(3, totalSteps, "⚙️ ", "Loading system daemons...")

	if !servctl2Running {
		printDetail(fmt.Sprintf("Loading servctl2 plist: %s", servctl2Plist))
		if _, err := os.Stat(servctl2Plist); os.IsNotExist(err) {
			printDetail("⚠️  servctl2 plist not found, skipping (FortiClient may need reinstall)")
		} else {
			if err := runSudoCommand("Loading servctl2 daemon", "launchctl", "load", servctl2Plist); err != nil {
				printDetail("⚠️  Failed to load servctl2 (may already be loaded): " + err.Error())
			} else {
				printDetail("✓ servctl2 plist loaded")
			}

			printDetail("Starting servctl2 service...")
			if err := runSudoCommand("Starting servctl2 service", "launchctl", "start", servctl2ServiceLabel); err != nil {
				printDetail("⚠️  Failed to start servctl2: " + err.Error())
			} else {
				printDetail("✓ servctl2 service started")
			}
		}
	} else {
		printDetail("✓ servctl2 is already running, skipping")
	}

	if !privilegedHelperRunning {
		printDetail(fmt.Sprintf("Loading PrivilegedHelper plist: %s", privilegedHelperPlist))
		if _, err := os.Stat(privilegedHelperPlist); os.IsNotExist(err) {
			printDetail("⚠️  PrivilegedHelper plist not found, skipping (FortiClient may need reinstall)")
		} else {
			if err := runSudoCommand("Loading PrivilegedHelper daemon", "launchctl", "load", privilegedHelperPlist); err != nil {
				printDetail("⚠️  Failed to load PrivilegedHelper (may already be loaded): " + err.Error())
			} else {
				printDetail("✓ PrivilegedHelper plist loaded")
			}
		}
	} else {
		printDetail("✓ PrivilegedHelper is already running, skipping")
	}

	// ═══════════════ Step 4: Restart FortiClient ═══════════════
	printStep(4, totalSteps, "🔄", "Restarting FortiClient...")

	printDetail("Stopping FortiClient processes...")
	for _, proc := range strings.Fields(fortiClientProcesses) {
		cmd := newCommand("killall", proc)
		cmd.Run() // ignore errors — process may not be running
	}
	printDetail("✓ FortiClient processes stopped")

	printDetail("Starting FortiClient...")
	openCmd := newCommand("open", fortiClientApp)
	openCmd.Stdout = os.Stdout
	openCmd.Stderr = os.Stderr
	if err := openCmd.Run(); err != nil {
		return fmt.Errorf("failed to start FortiClient: %w", err)
	}
	printDetail("✓ FortiClient started")

	// ═══════════════ Step 5: Start FortiTray ═══════════════
	printStep(5, totalSteps, "🌐", "Starting FortiTray (SAML handler)...")

	if !fortiTrayRunning {
		printDetail("Waiting for FortiClient to initialize...")
		// Give FortiClient a moment to start up before launching FortiTray
		waitCmd := newCommand("sleep", "3")
		waitCmd.Run()

		if _, err := os.Stat(fortiTrayApp); os.IsNotExist(err) {
			printDetail("⚠️  FortiTray app not found at expected path")
			printDetail("    Path: " + fortiTrayApp)
			printDetail("    SAML login may not work. Consider reinstalling FortiClient.")
		} else {
			printDetail(fmt.Sprintf("Launching FortiTray: %s", fortiTrayApp))
			trayCmd := newCommand("open", fortiTrayApp)
			trayCmd.Stdout = os.Stdout
			trayCmd.Stderr = os.Stderr
			if err := trayCmd.Run(); err != nil {
				printDetail("⚠️  Failed to start FortiTray: " + err.Error())
				printDetail("    SAML login may not work.")
				printDetail("    Try enabling FortiClient in: System Settings → General → Login Items & Extensions")
			} else {
				printDetail("✓ FortiTray started")
			}
		}
	} else {
		printDetail("✓ FortiTray was already running")
	}

	// ═══════════════ Step 6: Verify ═══════════════
	printStep(6, totalSteps, "✅", "Verifying fix...")

	printDetail("Waiting for services to stabilize...")
	waitCmd := newCommand("sleep", "2")
	waitCmd.Run()

	servctl2OK := isProcessRunning("fctservctl2")
	privilegedHelperOK := isProcessRunning("PrivilegedHelper")
	fortiTrayOK := isProcessRunning("FortiTray")
	fortiClientOK := isProcessRunning("FortiClient")

	printDetail(fmt.Sprintf("FortiClient (GUI):          %s", statusEmoji(fortiClientOK)))
	printDetail(fmt.Sprintf("fctservctl2 (core daemon):  %s", statusEmoji(servctl2OK)))
	printDetail(fmt.Sprintf("PrivilegedHelper:           %s", statusEmoji(privilegedHelperOK)))
	printDetail(fmt.Sprintf("FortiTray (SAML handler):   %s", statusEmoji(fortiTrayOK)))

	allOK := servctl2OK && privilegedHelperOK && fortiTrayOK && fortiClientOK
	if allOK {
		printSection("🎉", "FortiClient VPN fixed successfully!")
		printDetail("All services are running. You should now be able to:")
		printDetail("  • See the FortiClient interface normally")
		printDetail("  • Use SAML login to connect to VPN")
	} else {
		fmt.Println()
		printDetail("⚠️  Some services may still not be running.")
		printDetail("Troubleshooting tips:")
		if !servctl2OK || !privilegedHelperOK {
			printDetail("  • Blank screen: Try reinstalling FortiClient VPN via 'tdu upgrade'")
		}
		if !fortiTrayOK {
			printDetail("  • SAML login: Enable FortiClient in System Settings → General → Login Items & Extensions")
		}
		fmt.Println()
	}

	return nil
}

func isProcessRunning(name string) bool {
	out, err := newCommand("pgrep", "-x", name).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func statusEmoji(running bool) string {
	if running {
		return "✅ Running"
	}
	return "❌ Not running"
}

func getFortiClientVersion() string {
	out, err := newCommand("defaults", "read", fortiClientVersionPlist, "CFBundleShortVersionString").CombinedOutput()
	if err != nil {
		return ""
	}
	version := strings.TrimSpace(string(out))
	if version == "" || strings.Contains(version, "does not exist") {
		return ""
	}
	return version
}
