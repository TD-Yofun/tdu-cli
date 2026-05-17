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

	candidates, err := listSortedMpkgAssets(release.Assets)
	if err != nil {
		return err
	}
	printDetail(fmt.Sprintf("Found %d installable version(s)", len(candidates)))

	// ═══════════════ Step 3: Select version ═══════════════
	printStep(3, "🎯", "Select version to install...")
	items := make([]string, len(candidates))
	for i, c := range candidates {
		label := c.version
		if i == 0 {
			label += "  (latest)"
		}
		if installed && c.version == localVersion {
			label += "  [installed]"
		}
		items[i] = label
	}
	selectPrompt := promptui.Select{
		Label: "  Select FortiClient VPN version",
		Items: items,
		Size:  8,
		Templates: &promptui.SelectTemplates{
			Label:    "  {{ . }}",
			Active:   "  ➜ {{ . | cyan }}",
			Inactive: "    {{ . }}",
			Selected: "  ✅ {{ . | green }}",
		},
	}
	selIdx, _, err := selectPrompt.Run()
	if err != nil {
		fmt.Println()
		fmt.Println("  ❌ Operation cancelled by user.")
		fmt.Println()
		return nil
	}
	selected := candidates[selIdx]
	selectedVersion := selected.version
	selectedAsset := selected.asset
	printDetail(fmt.Sprintf("Asset: %s (%s)", selectedAsset.Name, formatBytes(selectedAsset.Size)))

	action := "install"
	if installed {
		switch cmp := compareVersions(localVersion, selectedVersion); {
		case cmp == 0:
			action = "reinstall"
			printDetail(fmt.Sprintf("Selected version (%s) is already installed", selectedVersion))
		case cmp > 0:
			action = "downgrade"
			printDetail(fmt.Sprintf("Downgrade: %s → %s", localVersion, selectedVersion))
		default:
			action = "upgrade"
			printDetail(fmt.Sprintf("Upgrade: %s → %s", localVersion, selectedVersion))
		}
	} else {
		printDetail(fmt.Sprintf("Will install version: %s", selectedVersion))
	}

	fmt.Println()
	confirmPrompt := promptui.Prompt{
		Label:     fmt.Sprintf("  Do you want to %s FortiClient VPN (%s)", action, selectedVersion),
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
	printDetail(fmt.Sprintf("Target asset: %s", selectedAsset.Name))
	printDetail(fmt.Sprintf("Asset ID: %d, Size: %s", selectedAsset.ID, formatBytes(selectedAsset.Size)))

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	cacheDir := filepath.Join(homeDir, fortiClientCacheBase, selectedVersion)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	downloadPath := filepath.Join(cacheDir, selectedAsset.Name)

	cached := false
	if fi, err := os.Stat(downloadPath); err == nil && fi.Size() == selectedAsset.Size {
		cached = true
		printDetail(fmt.Sprintf("Using cached download: %s (%s)", downloadPath, formatBytes(fi.Size())))
		printDetail("✓ Cached file size matches expected size, skipping download")
	}

	if !cached {
		printDetail(fmt.Sprintf("Downloading to: %s", downloadPath))
		if err := downloadReleaseAsset(token, fortiClientRepo, selectedAsset.ID, selectedAsset.Size, downloadPath); err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
		fi, _ := os.Stat(downloadPath)
		printDetail(fmt.Sprintf("Download complete: %s (%s)", downloadPath, formatBytes(fi.Size())))
		printDetail("✓ Download complete")
	}

	// ═══════════════ Step 5: Install ═══════════════
	printStep(5, "📦", "Installing FortiClient VPN...")
	printDetail(fmt.Sprintf("Installing %s ...", selectedAsset.Name))
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

type mpkgCandidate struct {
	asset   *ghAsset
	version string
	parts   []int
}

// listSortedMpkgAssets returns all forticlient-vpn-*.mpkg assets from a release
// sorted in descending version order (latest first).
func listSortedMpkgAssets(assets []ghAsset) ([]mpkgCandidate, error) {
	re := regexp.MustCompile(`^forticlient-vpn-([\d]+(?:\.[\d]+)+)\.mpkg$`)

	var candidates []mpkgCandidate
	for i := range assets {
		matches := re.FindStringSubmatch(assets[i].Name)
		if len(matches) >= 2 {
			parts, err := parseVersionParts(matches[1])
			if err != nil {
				continue
			}
			candidates = append(candidates, mpkgCandidate{
				asset:   &assets[i],
				version: matches[1],
				parts:   parts,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no forticlient-vpn-*.mpkg asset found in release")
	}

	sort.Slice(candidates, func(i, j int) bool {
		return compareVersionSlices(candidates[i].parts, candidates[j].parts) > 0
	})

	return candidates, nil
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
