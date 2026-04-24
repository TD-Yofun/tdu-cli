package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/manifoldco/promptui"
)

const (
	tdCliRepo       = "Talkdesk/td-cli"
	tdCliBinary     = "td"
	tdCliInstallDir = "/usr/local/bin"
	tdCliCacheBase  = ".cache/tdu/td-cli"
)

func upgradeTdCli() error {
	token, err := requireGitHubToken()
	if err != nil {
		return err
	}

	printSection("🔧", "td-cli Upgrade")
	printDetail(fmt.Sprintf("Repository : https://github.com/%s", tdCliRepo))
	printDetail(fmt.Sprintf("Install dir: %s", tdCliInstallDir))

	// ═══════════════ Step 1: Check local version ═══════════════
	printStep(1, "🔍", "Checking local td-cli version...")
	localVersion, installed := getLocalTdCliVersion()
	if installed {
		printDetail(fmt.Sprintf("Local td-cli version: %s", localVersion))
	} else {
		printDetail("td-cli is not currently installed on this system")
	}

	// ═══════════════ Step 2: Fetch latest release ═══════════════
	printStep(2, "🌐", "Fetching latest release info from GitHub...")
	printDetail(fmt.Sprintf("API: https://api.github.com/repos/%s/releases/latest", tdCliRepo))
	release, err := fetchGitHubRelease(token, fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", tdCliRepo))
	if err != nil {
		return fmt.Errorf("failed to fetch latest release: %w", err)
	}
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	printDetail(fmt.Sprintf("Latest release version: %s (tag: %s)", latestVersion, release.TagName))
	printDetail(fmt.Sprintf("Release contains %d asset(s)", len(release.Assets)))

	// ═══════════════ Step 3: Compare versions ═══════════════
	printStep(3, "🔄", "Comparing versions...")
	if installed && localVersion == latestVersion {
		fmt.Println()
		fmt.Println("  ✅ td-cli is already up to date! (version: " + localVersion + ")")
		fmt.Println()
		return nil
	}

	action := "install"
	if installed {
		action = "upgrade"
		printDetail(fmt.Sprintf("Version change: %s → %s", localVersion, latestVersion))
	} else {
		printDetail(fmt.Sprintf("Will install version: %s", latestVersion))
	}

	fmt.Println()
	confirmPrompt := promptui.Prompt{
		Label:     fmt.Sprintf("  Do you want to %s td-cli to %s", action, latestVersion),
		IsConfirm: true,
	}
	_, err = confirmPrompt.Run()
	if err != nil {
		fmt.Println()
		fmt.Println("  ❌ Operation cancelled by user.")
		fmt.Println()
		return nil
	}
	fmt.Println()

	// ═══════════════ Step 4: Detect architecture & download ═══════════════
	printStep(4, "⬇️ ", "Downloading release asset...")

	assetName, err := getTdCliAssetName()
	if err != nil {
		return err
	}
	printDetail(fmt.Sprintf("System architecture: %s (%s)", runtime.GOARCH, runtime.GOOS))
	printDetail(fmt.Sprintf("Target asset: %s", assetName))

	// Find matching asset
	var targetAsset *ghAsset
	for i := range release.Assets {
		if release.Assets[i].Name == assetName {
			targetAsset = &release.Assets[i]
			break
		}
	}
	if targetAsset == nil {
		return fmt.Errorf("asset %s not found in release %s", assetName, latestVersion)
	}
	printDetail(fmt.Sprintf("Asset ID: %d, Size: %s", targetAsset.ID, formatBytes(targetAsset.Size)))

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	cacheDir := filepath.Join(homeDir, tdCliCacheBase, latestVersion)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	downloadPath := filepath.Join(cacheDir, assetName)

	// Check if a valid cached download already exists
	cached := false
	if fi, err := os.Stat(downloadPath); err == nil && fi.Size() == targetAsset.Size {
		cached = true
		printDetail(fmt.Sprintf("Using cached download: %s (%s)", downloadPath, formatBytes(fi.Size())))
		printDetail("✓ Cached file size matches expected size, skipping download")
	}

	if !cached {
		printDetail(fmt.Sprintf("Downloading to: %s", downloadPath))
		if err := downloadReleaseAsset(token, tdCliRepo, targetAsset.ID, targetAsset.Size, downloadPath); err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
		fi, _ := os.Stat(downloadPath)
		printDetail(fmt.Sprintf("Download complete: %s (%s)", downloadPath, formatBytes(fi.Size())))

		// Validate download
		if fi.Size() < 1024 {
			content, _ := os.ReadFile(downloadPath)
			printDetail(fmt.Sprintf("⚠️  File content: %s", string(content)))
			return fmt.Errorf("downloaded file is too small (%d bytes), possibly not a valid binary. Check your token permissions", fi.Size())
		}
		printDetail("✓ Download size looks valid")
	}

	// ═══════════════ Step 5: Extract & install ═══════════════
	printStep(5, "📦", "Extracting and installing...")

	printDetail(fmt.Sprintf("Extracting %s ...", assetName))
	extractCmd := newCommand("tar", "-xzf", downloadPath, "-C", cacheDir)
	extractCmd.Stdout = os.Stdout
	extractCmd.Stderr = os.Stderr
	if err := extractCmd.Run(); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}
	printDetail("✓ Archive extracted successfully")

	tdBinPath, err := findBinary(cacheDir, tdCliBinary)
	if err != nil {
		return err
	}
	printDetail(fmt.Sprintf("Found binary: %s", tdBinPath))

	destPath := filepath.Join(tdCliInstallDir, tdCliBinary)
	printDetail(fmt.Sprintf("Installing to: %s", destPath))
	printDetail("(sudo required — you may be prompted for your password)")
	fmt.Println()

	// Move binary
	if err := runSudoCommand("Moving binary to "+destPath, "mv", tdBinPath, destPath); err != nil {
		return err
	}

	// chmod +x
	printDetail("Setting executable permission: chmod +x " + destPath)
	if err := runSudoCommand("Setting permissions", "chmod", "+x", destPath); err != nil {
		return err
	}
	printDetail("✓ Executable permission set")

	// Remove quarantine
	printDetail("Removing macOS quarantine attribute: xattr -d com.apple.quarantine " + destPath)
	xattrCmd := newCommand("sudo", "xattr", "-d", "com.apple.quarantine", destPath)
	xattrCmd.Stdout = os.Stdout
	xattrCmd.Stderr = os.Stderr
	if err := xattrCmd.Run(); err != nil {
		printDetail("⚠️  Quarantine attribute not present or already removed (this is OK)")
	} else {
		printDetail("✓ Quarantine attribute removed")
	}

	// ═══════════════ Step 6: Verify ═══════════════
	printStep(6, "✅", "Verifying installation...")

	newVersion, ok := getLocalTdCliVersion()
	if ok {
		printDetail(fmt.Sprintf("Installed version: %s", newVersion))
		printSection("🎉", fmt.Sprintf("td-cli %sd successfully! (%s)", action, newVersion))
	} else {
		printDetail("⚠️  Could not verify version, but installation completed")
		printDetail("Try running: td -v")
	}

	// Clean up cache only after successful installation
	fmt.Printf("  🧹 Cleaning up cache: %s\n", cacheDir)
	os.RemoveAll(cacheDir)

	return nil
}

func getLocalTdCliVersion() (string, bool) {
	out, err := newCommand("td", "-v").CombinedOutput()
	if err != nil {
		return "", false
	}
	re := regexp.MustCompile(`(?i)talkdesk\s+command-line\s+v?([\d]+\.[\d]+\.[\d]+)`)
	matches := re.FindStringSubmatch(string(out))
	if len(matches) >= 2 {
		return matches[1], true
	}
	// Fallback: try to find any semver-like pattern in last line
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	lastLine := lines[len(lines)-1]
	reFallback := regexp.MustCompile(`v?([\d]+\.[\d]+\.[\d]+)`)
	m := reFallback.FindStringSubmatch(lastLine)
	if len(m) >= 2 {
		return m[1], true
	}
	return strings.TrimSpace(string(out)), strings.TrimSpace(string(out)) != ""
}

func getTdCliAssetName() (string, error) {
	arch := runtime.GOARCH
	switch arch {
	case "arm64":
		return "td-darwin-arm64.tar.gz", nil
	case "amd64":
		return "td-darwin-amd64.tar.gz", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", arch)
	}
}
