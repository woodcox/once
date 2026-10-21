package version

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubReleasesURL  = "https://api.github.com/repos/woodcox/once/releases/latest"
	checksumsAssetName = "checksums.txt"
	httpTimeout        = 30 * time.Second
	updateTempFile     = ".once-update-tmp"
)

type Updater struct {
	currentVersion string
	apiURL         string
	client         *http.Client
	githubToken    string
	execPath       func() (string, error)
}

func NewUpdater() *Updater {
	u := newUpdater(Version, githubReleasesURL, &http.Client{Timeout: httpTimeout})
	u.githubToken = os.Getenv("GITHUB_TOKEN")
	u.execPath = os.Executable
	return u
}

func newUpdater(currentVersion, apiURL string, client *http.Client) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		apiURL:         apiURL,
		client:         client,
		execPath:       os.Executable,
	}
}

func (u *Updater) UpdateBinary() error {
	rel, err := u.fetchRelease()
	if err != nil {
		return err
	}

	if rel.TagName == u.currentVersion {
		fmt.Printf("You already have the latest version (%s)\n", u.currentVersion)
		return nil
	}

	assetName := fmt.Sprintf("once-%s-%s", runtime.GOOS, runtime.GOARCH)
	var asset *asset
	for i := range rel.Assets {
		if rel.Assets[i].Name == assetName {
			asset = &rel.Assets[i]
			break
		}
	}
	if asset == nil {
		return fmt.Errorf("no release asset found for %s", assetName)
	}

	checksumsURL := rel.checksumsAssetURL()
	if checksumsURL == "" {
		return fmt.Errorf("release %s has no %s asset; refusing to install %s without integrity check", rel.TagName, checksumsAssetName, assetName)
	}

	expected, err := u.lookupChecksum(checksumsURL, assetName)
	if err != nil {
		return err
	}

	fmt.Printf("Updating once from %s to %s...\n", u.currentVersion, rel.TagName)

	body, err := u.downloadBinary(asset.downloadURL())
	if err != nil {
		return err
	}

	if err := verifyChecksum(body, expected, assetName); err != nil {
		return err
	}

	execPath, err := u.execPath()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}

	tmpFile := filepath.Join(filepath.Dir(execPath), updateTempFile)
	if err := os.WriteFile(tmpFile, body, 0o755); err != nil {
		return fmt.Errorf("writing update to temp file: %w", err)
	}

	if err := u.replaceBinary(execPath, tmpFile); err != nil {
		return err
	}

	fmt.Println("Update complete.")
	return nil
}

// Private

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	// BrowserURL is the public download link. The API URL only returns the
	// asset bytes when the request accepts application/octet-stream, so
	// prefer this whenever GitHub supplies it.
	BrowserURL string `json:"browser_download_url"`
}

// downloadURL prefers the public asset link and falls back to the API URL for
// releases (or tests) that only provide that.
func (a asset) downloadURL() string {
	if a.BrowserURL != "" {
		return a.BrowserURL
	}
	return a.URL
}

type progressReader struct {
	r     io.Reader
	total int64 // -1 if unknown
	read  int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	if p.total > 0 {
		fmt.Printf("\r  %.1f / %.1f MB (%d%%)", float64(p.read)/1e6, float64(p.total)/1e6, p.read*100/p.total)
	} else {
		fmt.Printf("\r  %.1f MB", float64(p.read)/1e6)
	}
	return n, err
}

func (r *release) checksumsAssetURL() string {
	for _, a := range r.Assets {
		if a.Name == checksumsAssetName {
			return a.downloadURL()
		}
	}
	return ""
}

func (u *Updater) get(url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if u.githubToken != "" {
		req.Header.Set("Authorization", "token "+u.githubToken)
	}
	return u.client.Do(req)
}

func (u *Updater) fetchRelease() (*release, error) {
	resp, err := u.get(u.apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetching release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching release: unexpected status %d", resp.StatusCode)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}

	return &rel, nil
}

func (u *Updater) downloadBinary(url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("downloading binary: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	if u.githubToken != "" {
		req.Header.Set("Authorization", "token "+u.githubToken)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading binary: unexpected status %d", resp.StatusCode)
	}

	pr := &progressReader{r: resp.Body, total: resp.ContentLength}
	body, err := io.ReadAll(pr)
	fmt.Println()
	if err != nil {
		return nil, fmt.Errorf("reading binary: %w", err)
	}
	return body, nil
}

func (u *Updater) lookupChecksum(checksumsURL, assetName string) (string, error) {
	body, err := u.downloadChecksums(checksumsURL)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", checksumsAssetName, err)
	}

	digest, ok := parseChecksumEntry(body, assetName)
	if !ok {
		return "", fmt.Errorf("%s has no entry for %s; refusing to install an unchecked binary", checksumsAssetName, assetName)
	}
	return digest, nil
}

func (u *Updater) downloadChecksums(url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// The API asset URL returns JSON metadata without this header; the
	// public browser_download_url doesn't care either way.
	req.Header.Set("Accept", "application/octet-stream")
	if u.githubToken != "" {
		req.Header.Set("Authorization", "token "+u.githubToken)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (u *Updater) replaceBinary(execPath, newPath string) error {
	oldPath := execPath + ".old"

	if err := os.Rename(execPath, oldPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	if err := os.Rename(newPath, execPath); err != nil {
		// Attempt to restore from backup
		os.Rename(oldPath, execPath)
		return fmt.Errorf("replacing binary: %w", err)
	}

	os.Remove(oldPath)
	return nil
}

// Helpers

// parseChecksumEntry reads the body of a checksums.txt file and returns the
// hex SHA-256 for the named asset. Each line is "<sha256>  <asset-name>" (two
// spaces, the format `sha256sum` emits by default). Lines starting with '#',
// blank lines, and lines that don't reference assetName are ignored.
func parseChecksumEntry(body []byte, assetName string) (string, bool) {
	want := strings.TrimSpace(assetName)
	for line := range strings.SplitSeq(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if fields[1] == want || fields[1] == "./"+want || fields[1] == "*"+want {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

func verifyChecksum(body []byte, expected, assetName string) error {
	sum := sha256.Sum256(body)
	got := hex.EncodeToString(sum[:])
	want := strings.ToLower(strings.TrimSpace(expected))
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s; refusing to install an unchecked binary", assetName, want, got)
	}
	return nil
}
