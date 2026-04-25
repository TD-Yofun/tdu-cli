package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/TD-Yofun/tdu-cli/cmd/utils"
	"github.com/manifoldco/promptui"
)

const (
	reportRepo              = "TD-Yofun/tdu-cli"
	fortiClientAppPath      = "/Applications/FortiClient.app"
	fortiClientInfoPlist    = "/Applications/FortiClient.app/Contents/Info.plist"
	fortiClientUserLogDir   = "Library/Logs/FortiClient"
	fortiClientSystemLogDir = "/Library/Application Support/Fortinet/FortiClient/Logs"
	maxLogLines             = 50
)

// Package-level aliases for shared utilities.
var newCommand = utils.NewCommand
var printSection = utils.PrintSection
var printStep = utils.PrintStep
var printDetail = utils.PrintDetail
var requireGitHubToken = utils.RequireGitHubToken

// logFile represents a log file to collect.
type logFile struct {
	Label string
	Path  string
	Sudo  bool
}

func reportFortiClientVPN() error {
	token, err := requireGitHubToken()
	if err != nil {
		return err
	}

	printSection("📋", "FortiClient VPN Issue Report")

	totalSteps := 5

	// ═══════════════ Step 1: Collect app info ═══════════════
	printStep(1, totalSteps, "🔍", "Collecting FortiClient VPN info...")

	info := collectFortiClientInfo()
	for _, line := range info {
		printDetail(line)
	}

	// ═══════════════ Step 2: Collect service status ═══════════════
	printStep(2, totalSteps, "⚙️ ", "Checking service status...")

	serviceStatus := collectServiceStatus()
	for _, line := range serviceStatus {
		printDetail(line)
	}

	// ═══════════════ Step 3: Collect logs ═══════════════
	printStep(3, totalSteps, "📄", "Collecting error logs...")

	needsSudo := checkNeedsSudo()
	if needsSudo {
		fmt.Println()
		printDetail("⚠️  Some log files require sudo (administrator) privileges to read:")
		printDetail("  • /Library/Application Support/Fortinet/FortiClient/Logs/")
		fmt.Println()

		confirmPrompt := promptui.Prompt{
			Label:     "  Grant sudo access to read system logs",
			IsConfirm: true,
		}
		_, err := confirmPrompt.Run()
		if err != nil {
			printDetail("Skipping system logs (user declined sudo)")
			needsSudo = false
		}
		fmt.Println()
	}

	logs := collectLogs(needsSudo)
	printDetail(fmt.Sprintf("Collected %d log source(s)", len(logs)))

	// ═══════════════ Step 4: Check existing issues & compose ═══════════════
	printStep(4, totalSteps, "✏️ ", "Checking for existing open issues...")

	username, err := getGitHubUsername(token)
	if err != nil {
		printDetail("⚠️  Could not fetch GitHub username: " + err.Error())
		printDetail("Will create a new issue instead")
	}

	var existingIssue *ghIssueResponse
	if username != "" {
		existingIssue, err = findOpenIssue(token, username)
		if err != nil {
			printDetail("⚠️  Could not search existing issues: " + err.Error())
		} else if existingIssue != nil {
			printDetail(fmt.Sprintf("Found existing open issue #%d: %s", existingIssue.Number, existingIssue.HTMLURL))
		} else {
			printDetail("No existing open issue found, will create a new one")
		}
	}

	title, body := composeIssue(info, serviceStatus, logs)

	// Show preview and confirm
	fmt.Println()
	if existingIssue != nil {
		printDetail(fmt.Sprintf("═══════════════ Comment Preview (append to #%d) ═══════════════", existingIssue.Number))
	} else {
		printDetail("═══════════════ Issue Preview ═══════════════")
		printDetail(fmt.Sprintf("Title: %s", title))
	}
	fmt.Println()
	// Show truncated preview
	previewLines := strings.Split(body, "\n")
	maxPreview := 30
	for i, line := range previewLines {
		if i >= maxPreview {
			fmt.Printf("        ... (%d more lines)\n", len(previewLines)-maxPreview)
			break
		}
		fmt.Printf("        %s\n", line)
	}
	fmt.Println()
	printDetail("═══════════════════════════════════════════")
	fmt.Println()

	var confirmLabel string
	if existingIssue != nil {
		confirmLabel = fmt.Sprintf("  Add comment to issue #%d", existingIssue.Number)
	} else {
		confirmLabel = "  Create this issue on GitHub"
	}

	confirmPrompt := promptui.Prompt{
		Label:     confirmLabel,
		IsConfirm: true,
	}
	_, err = confirmPrompt.Run()
	if err != nil {
		fmt.Println()
		fmt.Println("  ❌ Operation cancelled.")
		fmt.Println()
		return nil
	}
	fmt.Println()

	// ═══════════════ Step 5: Submit ═══════════════
	printStep(5, totalSteps, "🚀", "Submitting to GitHub...")

	var resultURL string
	if existingIssue != nil {
		resultURL, err = addIssueComment(token, existingIssue.Number, body)
		if err != nil {
			return fmt.Errorf("failed to add comment: %w", err)
		}
		printSection("🎉", fmt.Sprintf("Comment added to issue #%d!", existingIssue.Number))
	} else {
		resultURL, err = createGitHubIssue(token, title, body)
		if err != nil {
			return fmt.Errorf("failed to create GitHub issue: %w", err)
		}
		printSection("🎉", "Issue created successfully!")
	}

	printDetail(fmt.Sprintf("URL: %s", resultURL))
	printDetail("Thank you for reporting the issue!")

	return nil
}

func collectFortiClientInfo() []string {
	var info []string

	// App version
	version := ""
	out, err := newCommand("defaults", "read", fortiClientInfoPlist, "CFBundleShortVersionString").CombinedOutput()
	if err == nil {
		version = strings.TrimSpace(string(out))
		info = append(info, fmt.Sprintf("FortiClient version: %s", version))
	} else {
		info = append(info, "FortiClient version: Not installed or unknown")
	}

	// macOS version
	out, err = newCommand("sw_vers", "-productVersion").CombinedOutput()
	if err == nil {
		info = append(info, fmt.Sprintf("macOS version:       %s", strings.TrimSpace(string(out))))
	}

	// Architecture
	info = append(info, fmt.Sprintf("Architecture:        %s", runtime.GOARCH))

	// Hostname (anonymized)
	hostname, _ := os.Hostname()
	if hostname != "" {
		info = append(info, fmt.Sprintf("Hostname:            %s", hostname))
	}

	return info
}

func collectServiceStatus() []string {
	var status []string

	processes := []struct {
		Name  string
		Label string
	}{
		{"FortiClient", "FortiClient (GUI)"},
		{"fctservctl2", "fctservctl2 (core daemon)"},
		{"FortiTray", "FortiTray (SAML handler)"},
	}

	for _, p := range processes {
		running := isProcessRunning(p.Name)
		icon := "✅"
		state := "Running"
		if !running {
			icon = "❌"
			state = "Not running"
		}
		status = append(status, fmt.Sprintf("%s %s: %s", icon, p.Label, state))
	}

	// PrivilegedHelper - check via launchctl
	loaded := isDaemonLoaded("com.fortinet.forticlient.macos.PrivilegedHelper")
	icon := "✅"
	state := "Loaded"
	if !loaded {
		icon = "❌"
		state = "Not loaded"
	}
	status = append(status, fmt.Sprintf("%s PrivilegedHelper (on-demand): %s", icon, state))

	return status
}

func checkNeedsSudo() bool {
	// Check if system log directory is readable without sudo
	entries, err := os.ReadDir(fortiClientSystemLogDir)
	if err != nil {
		return true
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			path := filepath.Join(fortiClientSystemLogDir, e.Name())
			f, err := os.Open(path)
			if err != nil {
				return true
			}
			f.Close()
			return false // At least one file is readable
		}
	}
	return false
}

func collectLogs(withSudo bool) map[string]string {
	logs := make(map[string]string)

	homeDir, _ := os.UserHomeDir()

	// User-level logs (no sudo needed)
	userLogs := []logFile{
		{"FortiClient main.log", filepath.Join(homeDir, fortiClientUserLogDir, "main.log"), false},
		{"FortiClient renderer.log", filepath.Join(homeDir, fortiClientUserLogDir, "renderer.log"), false},
	}

	// System-level logs (may need sudo)
	systemLogs := []logFile{
		{"servctl.log", filepath.Join(fortiClientSystemLogDir, "servctl.log"), true},
		{"privileged_helper.log", filepath.Join(fortiClientSystemLogDir, "privileged_helper.log"), true},
		{"vpn-provider.log", filepath.Join(fortiClientSystemLogDir, "vpn-provider.log"), true},
		{"fctc.log", filepath.Join(fortiClientSystemLogDir, "fctc.log"), true},
		{"update.log", filepath.Join(fortiClientSystemLogDir, "update.log"), true},
	}

	for _, lf := range userLogs {
		content := readLogTail(lf.Path, false)
		if content != "" {
			logs[lf.Label] = content
			printDetail(fmt.Sprintf("✓ %s (%d lines)", lf.Label, countLines(content)))
		}
	}

	if withSudo {
		for _, lf := range systemLogs {
			content := readLogTail(lf.Path, true)
			if content != "" {
				logs[lf.Label] = content
				printDetail(fmt.Sprintf("✓ %s (%d lines)", lf.Label, countLines(content)))
			}
		}
	} else {
		// Try reading without sudo
		for _, lf := range systemLogs {
			content := readLogTail(lf.Path, false)
			if content != "" {
				logs[lf.Label] = content
				printDetail(fmt.Sprintf("✓ %s (%d lines)", lf.Label, countLines(content)))
			} else {
				printDetail(fmt.Sprintf("⚠️  %s (not readable, skipped)", lf.Label))
			}
		}
	}

	return logs
}

func readLogTail(path string, useSudo bool) string {
	if useSudo {
		out, err := newCommand("sudo", "tail", "-n", fmt.Sprintf("%d", maxLogLines), path).CombinedOutput()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	out, err := newCommand("tail", "-n", fmt.Sprintf("%d", maxLogLines), path).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

func composeIssue(info, serviceStatus []string, logs map[string]string) (string, string) {
	const maxBodyLen = 60000 // GitHub limit is 65536, leave room for overhead
	now := time.Now().Format("2006-01-02 15:04")
	title := fmt.Sprintf("[FortiClient VPN] Issue report - %s", now)

	var b strings.Builder

	b.WriteString("## Environment\n\n")
	for _, line := range info {
		b.WriteString(fmt.Sprintf("- %s\n", line))
	}
	b.WriteString(fmt.Sprintf("- Report time: %s\n", now))

	b.WriteString("\n## Service Status\n\n")
	for _, line := range serviceStatus {
		b.WriteString(fmt.Sprintf("- %s\n", line))
	}

	b.WriteString("\n## Description\n\n")
	b.WriteString("<!-- Please describe the issue you are experiencing -->\n\n")

	headerLen := b.Len()

	if len(logs) > 0 {
		b.WriteString("\n## Logs\n\n")
		b.WriteString(fmt.Sprintf("_Last %d lines of each log file_\n\n", maxLogLines))

		for label, content := range logs {
			entry := fmt.Sprintf("<details>\n<summary>%s</summary>\n\n```\n%s\n```\n\n</details>\n\n", label, content)
			if b.Len()+len(entry) > maxBodyLen {
				// Truncate this log entry to fit
				remaining := maxBodyLen - b.Len() - 200 // reserve space for wrapper
				if remaining > 0 {
					truncated := truncateToLines(content, remaining)
					entry = fmt.Sprintf("<details>\n<summary>%s (truncated)</summary>\n\n```\n%s\n```\n\n</details>\n\n", label, truncated)
					b.WriteString(entry)
				}
				// Skip remaining logs
				b.WriteString(fmt.Sprintf("\n_Remaining logs omitted to stay within GitHub's %d character limit (header: %d chars, total: %d chars)_\n", maxBodyLen, headerLen, b.Len()))
				break
			}
			b.WriteString(entry)
		}
	}

	return title, b.String()
}

// truncateToLines truncates content to fit within maxBytes, keeping whole lines.
func truncateToLines(content string, maxBytes int) string {
	if len(content) <= maxBytes {
		return content
	}
	lines := strings.Split(content, "\n")
	var b strings.Builder
	for _, line := range lines {
		if b.Len()+len(line)+1 > maxBytes {
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

type ghIssueRequest struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
}

type ghIssueResponse struct {
	HTMLURL string `json:"html_url"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
}

type ghUserResponse struct {
	Login string `json:"login"`
}

type ghCommentResponse struct {
	HTMLURL string `json:"html_url"`
}

// getGitHubUsername fetches the authenticated user's login name.
func getGitHubUsername(token string) (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var user ghUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", err
	}
	return user.Login, nil
}

// findOpenIssue searches for an existing open issue created by the user with the forticlient-vpn label.
func findOpenIssue(token, username string) (*ghIssueResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/issues?state=open&labels=forticlient-vpn&creator=%s&per_page=1&sort=created&direction=desc",
		reportRepo, username)

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
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var issues []ghIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, err
	}

	if len(issues) == 0 {
		return nil, nil
	}
	return &issues[0], nil
}

// addIssueComment adds a comment to an existing GitHub issue.
func addIssueComment(token string, issueNumber int, body string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/comments", reportRepo, issueNumber)

	payload := struct {
		Body string `json:"body"`
	}{Body: body}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var commentResp ghCommentResponse
	if err := json.NewDecoder(resp.Body).Decode(&commentResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	return commentResp.HTMLURL, nil
}

func createGitHubIssue(token, title, body string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/issues", reportRepo)

	payload := ghIssueRequest{
		Title:  title,
		Body:   body,
		Labels: []string{"bug", "forticlient-vpn"},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal issue: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var issueResp ghIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&issueResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return issueResp.HTMLURL, nil
}

func isProcessRunning(name string) bool {
	out, err := newCommand("pgrep", "-x", name).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func isDaemonLoaded(label string) bool {
	cmd := newCommand("sudo", "launchctl", "list", label)
	err := cmd.Run()
	return err == nil
}
