package upgrade

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// parallelDownloadThreshold is the minimum file size that triggers parallel chunked download.
	parallelDownloadThreshold int64 = 5 * 1024 * 1024 // 5 MB
	// parallelDownloadWorkers is the number of concurrent range requests for large files.
	parallelDownloadWorkers = 4
	// downloadBufferSize is the per-read buffer size used by all download paths.
	downloadBufferSize = 256 * 1024 // 256 KB
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
// For files larger than parallelDownloadThreshold it issues N concurrent range
// requests against the resolved (post-redirect) URL for higher throughput.
func downloadReleaseAsset(token, repo string, assetID int, expectedSize int64, destPath string) error {
	finalURL, totalSize, supportsRange, err := resolveAssetURL(token, repo, assetID)
	if err != nil {
		// Could not resolve via Range probe; fall back to the legacy single-stream path.
		return downloadSingleStreamLegacy(token, repo, assetID, expectedSize, destPath)
	}
	if totalSize <= 0 {
		totalSize = expectedSize
	}

	if supportsRange && totalSize >= parallelDownloadThreshold {
		if perr := downloadParallel(finalURL, destPath, totalSize, parallelDownloadWorkers); perr == nil {
			return nil
		} else {
			fmt.Printf("\n        ⚠️  parallel download failed (%v), falling back to single stream\n", perr)
		}
	}

	return downloadStreamFromURL(finalURL, destPath, totalSize)
}

// resolveAssetURL issues a Range: bytes=0-0 probe against the GitHub assets API
// to capture (1) the post-redirect URL (typically a presigned S3 URL),
// (2) whether the server honors range requests, and (3) the total size.
// The Authorization header is stripped on redirect so S3 won't reject the request.
func resolveAssetURL(token, repo string, assetID int) (finalURL string, totalSize int64, supportsRange bool, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/assets/%d", repo, assetID)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header.Del("Authorization")
			return nil
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", 0, false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return "", 0, false, fmt.Errorf("probe returned %d", resp.StatusCode)
	}

	finalURL = resp.Request.URL.String()
	supportsRange = resp.StatusCode == http.StatusPartialContent

	if cr := resp.Header.Get("Content-Range"); cr != "" {
		if i := strings.LastIndex(cr, "/"); i > 0 && cr[i+1:] != "*" {
			totalSize, _ = strconv.ParseInt(cr[i+1:], 10, 64)
		}
	}
	if totalSize == 0 {
		totalSize = resp.ContentLength
	}
	return finalURL, totalSize, supportsRange, nil
}

// downloadParallel splits the file into N chunks and downloads them concurrently
// via Range requests against the presigned URL. All chunks share one progress tracker.
func downloadParallel(finalURL, destPath string, totalSize int64, workers int) error {
	if workers < 1 {
		workers = 1
	}
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	if err := out.Truncate(totalSize); err != nil {
		return err
	}

	tracker := newProgressTracker(totalSize)
	chunkSize := totalSize / int64(workers)

	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize - 1
		if i == workers-1 {
			end = totalSize - 1
		}
		wg.Add(1)
		go func(start, end int64) {
			defer wg.Done()
			if err := downloadChunk(finalURL, out, start, end, tracker); err != nil {
				errCh <- err
			}
		}(start, end)
	}
	wg.Wait()
	close(errCh)
	tracker.Finish()
	for e := range errCh {
		if e != nil {
			return e
		}
	}
	return nil
}

func downloadChunk(url string, out *os.File, start, end int64, tracker *progressTracker) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chunk [%d-%d] returned %d", start, end, resp.StatusCode)
	}

	buf := make([]byte, downloadBufferSize)
	offset := start
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.WriteAt(buf[:n], offset); werr != nil {
				return werr
			}
			offset += int64(n)
			tracker.Add(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	return nil
}

// downloadStreamFromURL streams a single GET from an already-resolved (presigned) URL.
func downloadStreamFromURL(url, destPath string, totalSize int64) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download returned %d: %s", resp.StatusCode, string(body))
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if totalSize <= 0 {
		totalSize = resp.ContentLength
	}
	return streamToFile(resp.Body, out, totalSize)
}

// downloadSingleStreamLegacy is the last-resort fallback: authenticated single GET
// against the assets API, used when the Range probe in resolveAssetURL fails.
func downloadSingleStreamLegacy(token, repo string, assetID int, expectedSize int64, destPath string) error {
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
	if totalSize <= 0 {
		totalSize = expectedSize
	}
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	return streamToFile(resp.Body, out, totalSize)
}

// streamToFile copies r → w with a progress tracker. If totalSize ≤ 0 the
// progress bar shows only the downloaded size and current speed.
func streamToFile(r io.Reader, w io.Writer, totalSize int64) error {
	tracker := newProgressTracker(totalSize)
	buf := make([]byte, downloadBufferSize)
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			tracker.Add(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	tracker.Finish()
	return nil
}

// ──────────────────────────── progress tracker ────────────────────────────

type progressTracker struct {
	mu         sync.Mutex
	total      int64
	downloaded int64
	started    time.Time
	samples    []progressSample
	lastDraw   time.Time
}

type progressSample struct {
	t time.Time
	n int64
}

func newProgressTracker(total int64) *progressTracker {
	return &progressTracker{total: total, started: time.Now()}
}

// Add records n freshly downloaded bytes and redraws the progress line if
// at least 100ms have passed since the last redraw. Safe for concurrent use.
func (p *progressTracker) Add(n int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.downloaded += n
	now := time.Now()
	p.samples = append(p.samples, progressSample{now, p.downloaded})

	// Keep only samples within the last 2 seconds for sliding-window speed.
	cutoff := now.Add(-2 * time.Second)
	keep := 0
	for i, s := range p.samples {
		if s.t.After(cutoff) {
			keep = i
			break
		}
	}
	if keep > 0 && keep < len(p.samples) {
		p.samples = p.samples[keep:]
	}

	if now.Sub(p.lastDraw) >= 100*time.Millisecond {
		p.lastDraw = now
		p.drawLocked()
	}
}

// Finish redraws the final state and emits a newline.
func (p *progressTracker) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.drawLocked()
	fmt.Println()
}

func (p *progressTracker) drawLocked() {
	speed := p.speedLocked()
	if p.total > 0 {
		percent := int(float64(p.downloaded) / float64(p.total) * 100)
		if percent > 100 {
			percent = 100
		}
		eta := "--"
		if speed > 0 && p.downloaded < p.total {
			remaining := float64(p.total-p.downloaded) / speed
			eta = formatDuration(time.Duration(remaining * float64(time.Second)))
		}
		fmt.Printf("\r        %s %3d%% (%s / %s)  %s/s  ETA %s   ",
			progressBar(percent, 30), percent,
			formatBytes(p.downloaded), formatBytes(p.total),
			formatBytes(int64(speed)), eta)
	} else {
		fmt.Printf("\r        downloading... %s  %s/s   ",
			formatBytes(p.downloaded), formatBytes(int64(speed)))
	}
}

func (p *progressTracker) speedLocked() float64 {
	if len(p.samples) >= 2 {
		first := p.samples[0]
		last := p.samples[len(p.samples)-1]
		dt := last.t.Sub(first.t).Seconds()
		if dt > 0 {
			return float64(last.n-first.n) / dt
		}
	}
	elapsed := time.Since(p.started).Seconds()
	if elapsed > 0 {
		return float64(p.downloaded) / elapsed
	}
	return 0
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	default:
		return fmt.Sprintf("%dh%02dm", s/3600, (s%3600)/60)
	}
}

func progressBar(percent, width int) string {
	filled := width * percent / 100
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return "[" + bar + "]"
}
