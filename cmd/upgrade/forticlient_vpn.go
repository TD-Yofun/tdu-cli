package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/manifoldco/promptui"
)

const (
	fortiClientRepo       = "TD-Yofun/tdu-cli"
	fortiClientReleaseTag = "forticlient-vpn"
	fortiClientCacheBase  = ".cache/tdu/forticlient-vpn"
)

func upgradeFortiClientVPN() error {
	token, err := requireGitHubToken()
	if err != nil {
		return err
	}

	printSection("🛡️", "FortiClient VPN Upgrade")
	printDetail(fmt.Sprintf("Repository : https://github.com/%s", fortiClientRepo))
	printDetail(fmt.Sprintf("Release tag: %s", fortiClientReleaseTag))

	// ═══════════════ Step 1: Check local version ═══════════════
	printStep(1, "🔍", "Checking local FortiClient VPN version...")
	localVersion, installed := getLocalFortiClientVersion()
	if installed {
		printDetail(fmt.Sprintf("Local FortiClient VPN version: %s", localVersion))
	} else {
		printDetail("FortiClient VPN is not currently installed on this system")
	}

	// ═══════════════ Step 2: Fetch release info ═══════════════
	printStep(2, "🌐", "Fetching release info from GitHub...")
	printDetail(fmt.Sprintf("API: https://api.github.com/repos/%s/releases/tags/%s", fortiClientRepo, fortiClientReleaseTag))
	release, err := fetchGitHubRelease(token, fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", fortiClientRepo, fortiClientReleaseTag))
	if err != nil {
		return fmt.Errorf("failed to fetch release: %w", err)
	}
	printDetail(fmt.Sprintf("Release contains %d asset(s)", len(release.Assets)))

	latestAsset, latestVersion, err := findLatestMpkgAsset(release.Assets)
	if err != nil {
		return err
	}
	printDetail(fmt.Sprintf("Latest FortiClient VPN version: %s", latestVersion))
	printDetail(fmt.Sprintf("Asset: %s (%s)", latestAsset.Name, formatBytes(latestAsset.Size)))

	// ═══════════════ Step 3: Compare versions ═══════════════
	printStep(3, "🔄", "Comparing versions...")
	if installed && compareVersions(localVersion, latestVersion) >= 0 {
		fmt.Println()
		fmt.Println("  ✅ FortiClient VPN is already up to date! (version: " + localVersion + ")")
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
		Label:     fmt.Sprintf("  Do you want to %s FortiClient VPN to %s", action, latestVersion),
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

	// ═══════════════ Step 4: Download ═══════════════
	printStep(4, "⬇️ ", "Downloading FortiClient VPN installer...")
	printDetail(fmt.Sprintf("Target asset: %s", latestAsset.Name))
	printDetail(fmt.Sprintf("Asset ID: %d, Size: %s", latestAsset.ID, formatBytes(latestAsset.Size)))

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	cacheDir := filepath.Join(homeDir, fortiClientCacheBase, latestVersion)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	downloadPath := filepath.Join(cacheDir, latestAsset.Name)

	cached := false
	if fi, err := os.Stat(downloadPath); err == nil && fi.Size() == latestAsset.Size {
		cached = true
		printDetail(fmt.Sprintf("Using cached download: %s (%s)", downloadPath, formatBytes(fi.Size())))
		printDetail("✓ Cached file size matches expected size, skipping download")
	}

	if !cached {
		printDetail(fmt.Sprintf("Downloading to: %s", downloadPath))
		if err := downloadReleaseAsset(token, fortiClientRepo, latestAsset.ID, latestAsset.Size, downloadPath); err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
		fi, _ := os.Stat(downloadPath)
		printDetail(fmt.Sprintf("Download complete: %s (%s)", downloadPath, formatBytes(fi.Size())))
		printDetail("✓ Download complete")
	}

	// ═══════════════ Step 5: Install ═══════════════
	printStep(5, "📦", "Installing FortiClient VPN...")
	printDetail(fmt.Sprintf("Installing %s ...", latestAsset.Name))
	printDetail("(sudo required — you may be prompted for your password)")
	fmt.Println()

	if err := runSudoCommand("Installing FortiClient VPN", "installer", "-pkg", downloadPath, "-target", "/"); err != nil {
		return err
	}
	printDetail("✓ Installation completed")

	// ═══════════════ Step 6: Verify ═══════════════
	printStep(6, "✅", "Verifying installation...")
	newVersion, ok := getLocalFortiClientVersion()
	if ok {
		printDetail(fmt.Sprintf("Installed version: %s", newVersion))
		printSection("🎉", fmt.Sprintf("FortiClient VPN %sd successfully! (%s)", action, newVersion))
	} else {
		printDetail("⚠️  Could not verify version, but installation completed")
		printDetail("Try checking: /Applications/FortiClient.app")
	}

	// Clean up cache
	fmt.Printf("  🧹 Cleaning up cache: %s\n", cacheDir)
	os.RemoveAll(cacheDir)

	return nil
}

func getLocalFortiClientVersion() (string, bool) {
	out, err := newCommand("defaults", "read",
		"/Applications/FortiClient.app/Contents/Info.plist",
		"CFBundleShortVersionString").CombinedOutput()
	if err != nil {
		return "", false
	}
	version := strings.TrimSpace(string(out))
	if version == "" || strings.Contains(version, "does not exist") {
		return "", false
	}
	return version, true
}

func findLatestMpkgAsset(assets []ghAsset) (*ghAsset, string, error) {
	re := regexp.MustCompile(`^forticlient-vpn-([\d]+(?:\.[\d]+)+)\.mpkg$`)

	type versionedAsset struct {
		asset   *ghAsset
		version string
		parts   []int
	}

	var candidates []versionedAsset
	for i := range assets {
		matches := re.FindStringSubmatch(assets[i].Name)
		if len(matches) >= 2 {
			parts, err := parseVersionParts(matches[1])
			if err != nil {
				continue
			}
			candidates = append(candidates, versionedAsset{
				asset:   &assets[i],
				version: matches[1],
				parts:   parts,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("no forticlient-vpn-*.mpkg asset found in release")
	}

	sort.Slice(candidates, func(i, j int) bool {
		return compareVersionSlices(candidates[i].parts, candidates[j].parts) > 0
	})

	latest := candidates[0]
	return latest.asset, latest.version, nil
}

func parseVersionParts(v string) ([]int, error) {
	segs := strings.Split(v, ".")
	if len(segs) < 2 {
		return nil, fmt.Errorf("invalid version format: %s", v)
	}
	result := make([]int, len(segs))
	for i, p := range segs {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid version component: %s", p)
		}
		result[i] = n
	}
	return result, nil
}

func compareVersionSlices(a, b []int) int {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	for i := 0; i < maxLen; i++ {
		va, vb := 0, 0
		if i < len(a) {
			va = a[i]
		}
		if i < len(b) {
			vb = b[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

func compareVersions(a, b string) int {
	partsA, errA := parseVersionParts(a)
	partsB, errB := parseVersionParts(b)
	if errA != nil || errB != nil {
		return strings.Compare(a, b)
	}
	return compareVersionSlices(partsA, partsB)
}
