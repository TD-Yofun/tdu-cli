package upgrade

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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

// fetchGitHubRelease fetches a release from the GitHub API at the given URL.
func fetchGitHubRelease(token, url string) (*ghRelease, error) {
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

// downloadReleaseAsset downloads a GitHub release asset with a progress bar.
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
