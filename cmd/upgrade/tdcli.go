package upgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func printSection(emoji, title string) {
	fmt.Println()
	fmt.Printf("  %s ═══════════════════════════════════════════\n", emoji)
	fmt.Printf("  %s  %s\n", emoji, title)
	fmt.Printf("  %s ═══════════════════════════════════════════\n", emoji)
	fmt.Println()
}

func printStep(step int, emoji, msg string) {
	fmt.Printf("  [%d/6] %s %s\n", step, emoji, msg)
}

func printDetail(msg string) {
	fmt.Printf("        %s\n", msg)
}

func upgradeTdCli() error {
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
		return fmt.Errorf("HOMEBREW_GITHUB_API_TOKEN is not set")
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
	release, err := fetchLatestRelease(token)
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

func runSudoCommand(description string, args ...string) error {
	sudoArgs := append([]string{}, args...)
	cmd := newCommand("sudo", sudoArgs...)
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
			fmt.Println("    3. Manually install the binary:")
			fmt.Printf("       sudo mv <downloaded_binary> %s/%s\n", tdCliInstallDir, tdCliBinary)
			fmt.Println()
		}
		return fmt.Errorf("%s failed: %w", description, err)
	}
	return nil
}

func formatBytes(b int64) string {
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

func fetchLatestRelease(token string) (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", tdCliRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release JSON: %w", err)
	}
	return &release, nil
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

func downloadReleaseAsset(token string, repo string, assetID int, expectedSize int64, destPath string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/assets/%d", repo, assetID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download returned %d: %s", resp.StatusCode, string(body))
	}

	totalSize := resp.ContentLength
	if totalSize <= 0 && expectedSize > 0 {
		totalSize = expectedSize
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if totalSize > 0 {
		var downloaded int64
		buf := make([]byte, 32*1024)
		lastPercent := -1
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				_, writeErr := out.Write(buf[:n])
				if writeErr != nil {
					return writeErr
				}
				downloaded += int64(n)
				percent := int(float64(downloaded) / float64(totalSize) * 100)
				if percent != lastPercent {
					bar := progressBar(percent, 30)
					fmt.Printf("\r        %s %3d%% (%s / %s)", bar, percent, formatBytes(downloaded), formatBytes(totalSize))
					lastPercent = percent
				}
			}
			if readErr != nil {
				if readErr == io.EOF {
					break
				}
				return readErr
			}
		}
		fmt.Println()
	} else {
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			return err
		}
	}

	return nil
}

func progressBar(percent, width int) string {
	filled := width * percent / 100
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return "[" + bar + "]"
}

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
