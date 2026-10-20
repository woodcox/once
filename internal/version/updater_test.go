package version

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateBinary_AlreadyLatest(t *testing.T) {
	srv := releaseServer(t, "v1.0.0", nil)
	u := newUpdater("v1.0.0", srv.URL, srv.Client())

	err := u.UpdateBinary()
	require.NoError(t, err)
}

func TestUpdateBinary_CorrectChecksumInstalls(t *testing.T) {
	dir := t.TempDir()

	execPath := filepath.Join(dir, "once")
	require.NoError(t, os.WriteFile(execPath, []byte("old"), 0o755))

	fakeBinary := []byte("new binary content")
	assets := releaseAssetsWithChecksum(t, fakeBinary)
	srv := releaseServer(t, "v2.0.0", assets)
	u := newUpdater("v1.0.0", srv.URL, srv.Client())
	u.execPath = func() (string, error) { return execPath, nil }

	require.NoError(t, u.UpdateBinary())

	got, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, fakeBinary, got)

	_, err = os.Stat(execPath + ".old")
	assert.True(t, os.IsNotExist(err))
}

func TestUpdateBinary_MismatchedDigestAborts(t *testing.T) {
	dir := t.TempDir()

	execPath := filepath.Join(dir, "once")
	original := []byte("old binary")
	require.NoError(t, os.WriteFile(execPath, original, 0o755))

	fakeBinary := []byte("new binary content")
	badDigest := sha256Hex([]byte("not the binary"))
	checksumsBody := []byte(fmt.Sprintf("%s  %s\n", badDigest, assetNameForPlatform()))

	checksumsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(checksumsBody)
	}))
	t.Cleanup(checksumsSrv.Close)

	assetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fakeBinary)
	}))
	t.Cleanup(assetSrv.Close)

	assets := []asset{
		{Name: "checksums.txt", URL: checksumsSrv.URL + "/checksums.txt"},
		{Name: assetNameForPlatform(), URL: assetSrv.URL + "/" + assetNameForPlatform()},
	}
	srv := releaseServer(t, "v2.0.0", assets)
	u := newUpdater("v1.0.0", srv.URL, srv.Client())
	u.execPath = func() (string, error) { return execPath, nil }

	err := u.UpdateBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")

	got, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func TestUpdateBinary_MissingChecksumsAssetAborts(t *testing.T) {
	dir := t.TempDir()

	execPath := filepath.Join(dir, "once")
	original := []byte("old binary")
	require.NoError(t, os.WriteFile(execPath, original, 0o755))

	assetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("anything"))
	}))
	t.Cleanup(assetSrv.Close)

	assets := []asset{{Name: assetNameForPlatform(), URL: assetSrv.URL + "/" + assetNameForPlatform()}}
	srv := releaseServer(t, "v2.0.0", assets)
	u := newUpdater("v1.0.0", srv.URL, srv.Client())
	u.execPath = func() (string, error) { return execPath, nil }

	err := u.UpdateBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksums.txt")
	assert.Contains(t, err.Error(), "refusing to install")

	got, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func TestUpdateBinary_MissingChecksumsFileAborts(t *testing.T) {
	dir := t.TempDir()

	execPath := filepath.Join(dir, "once")
	original := []byte("old binary")
	require.NoError(t, os.WriteFile(execPath, original, 0o755))

	checksumsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	t.Cleanup(checksumsSrv.Close)

	assetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("anything"))
	}))
	t.Cleanup(assetSrv.Close)

	assets := []asset{
		{Name: "checksums.txt", URL: checksumsSrv.URL + "/checksums.txt"},
		{Name: assetNameForPlatform(), URL: assetSrv.URL + "/" + assetNameForPlatform()},
	}
	srv := releaseServer(t, "v2.0.0", assets)
	u := newUpdater("v1.0.0", srv.URL, srv.Client())
	u.execPath = func() (string, error) { return execPath, nil }

	err := u.UpdateBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching checksums.txt")

	got, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func TestUpdateBinary_MissingEntryForAssetAborts(t *testing.T) {
	dir := t.TempDir()

	execPath := filepath.Join(dir, "once")
	original := []byte("old binary")
	require.NoError(t, os.WriteFile(execPath, original, 0o755))

	wrongName := "once-other-amd64"
	checksumsBody := []byte(fmt.Sprintf("%s  %s\n", sha256Hex([]byte("anything")), wrongName))

	checksumsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(checksumsBody)
	}))
	t.Cleanup(checksumsSrv.Close)

	assetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("anything"))
	}))
	t.Cleanup(assetSrv.Close)

	assets := []asset{
		{Name: "checksums.txt", URL: checksumsSrv.URL + "/checksums.txt"},
		{Name: assetNameForPlatform(), URL: assetSrv.URL + "/" + assetNameForPlatform()},
	}
	srv := releaseServer(t, "v2.0.0", assets)
	u := newUpdater("v1.0.0", srv.URL, srv.Client())
	u.execPath = func() (string, error) { return execPath, nil }

	err := u.UpdateBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no entry for")
	assert.Contains(t, err.Error(), assetNameForPlatform())

	got, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func TestUpdateBinary_AssetNotFoundError(t *testing.T) {
	srv := releaseServer(t, "v2.0.0", nil)
	u := newUpdater("v1.0.0", srv.URL, srv.Client())

	err := u.UpdateBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no release asset found")
}

func TestUpdateBinary_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	u := newUpdater("v1.0.0", srv.URL, srv.Client())
	err := u.UpdateBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "once")
	require.NoError(t, os.WriteFile(execPath, []byte("old"), 0o755))

	newContent := []byte("new")
	tmpPath := filepath.Join(dir, "once-tmp")
	require.NoError(t, os.WriteFile(tmpPath, newContent, 0o755))

	u := newUpdater("v1.0.0", "", nil)
	err := u.replaceBinary(execPath, tmpPath)
	require.NoError(t, err)

	got, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, newContent, got)

	_, err = os.Stat(execPath + ".old")
	assert.True(t, os.IsNotExist(err))
}

func TestParseChecksumEntry(t *testing.T) {
	body := []byte(`# comment line
not a checksum line: just text
` + sha256Hex([]byte("payload")) + `  once-linux-amd64
` + sha256Hex([]byte("other")) + `  once-darwin-arm64
`)

	digest, ok := parseChecksumEntry(body, "once-linux-amd64")
	require.True(t, ok)
	assert.Equal(t, sha256Hex([]byte("payload")), digest)

	_, ok = parseChecksumEntry(body, "once-windows-amd64")
	assert.False(t, ok)
}

func TestVerifyChecksum(t *testing.T) {
	payload := []byte("payload")
	want := sha256Hex(payload)
	assert.NoError(t, verifyChecksum(payload, want, "asset"))

	got := sha256Hex([]byte("other"))
	err := verifyChecksum(payload, got, "asset")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

// Helpers

func releaseServer(t *testing.T, tag string, assets []asset) *httptest.Server {
	t.Helper()
	rel := release{TagName: tag, Assets: assets}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rel)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func assetNameForPlatform() string {
	return fmt.Sprintf("once-%s-%s", runtime.GOOS, runtime.GOARCH)
}

func releaseAssetsWithChecksum(t *testing.T, binary []byte) []asset {
	t.Helper()

	name := assetNameForPlatform()
	digest := sha256Hex(binary)

	checksumsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  %s\n", digest, name)
	}))
	t.Cleanup(checksumsSrv.Close)

	binSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(binary)
	}))
	t.Cleanup(binSrv.Close)

	return []asset{
		{Name: "checksums.txt", URL: checksumsSrv.URL + "/checksums.txt"},
		{Name: name, URL: binSrv.URL + "/" + name},
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
