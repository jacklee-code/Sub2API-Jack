//go:build unit

package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type jackApplyClient struct {
	version, dir string
	corrupt      bool
}

func (c *jackApplyClient) assets() []GitHubAsset {
	root := "https://github.com/" + githubRepo + "/releases/download/v" + c.version + "/"
	return []GitHubAsset{{Name: c.archiveName(), BrowserDownloadURL: root + c.archiveName()}, {Name: "checksums.txt", BrowserDownloadURL: root + "checksums.txt"}, {Name: "jack-release.json", BrowserDownloadURL: root + "jack-release.json"}}
}
func (c *jackApplyClient) archiveName() string {
	return fmt.Sprintf("sub2api_%s_%s_%s.tar.gz", c.version, runtime.GOOS, runtime.GOARCH)
}
func (c *jackApplyClient) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	return &GitHubRelease{TagName: "v" + c.version, HTMLURL: "https://github.com/" + githubRepo + "/releases/tag/v" + c.version, Assets: c.assets()}, nil
}
func (c *jackApplyClient) FetchRecentReleases(ctx context.Context, repo string, _ int) ([]*GitHubRelease, error) {
	r, e := c.FetchLatestRelease(ctx, repo)
	return []*GitHubRelease{r}, e
}
func (c *jackApplyClient) DownloadFile(_ context.Context, _ string, destination string, _ int64) error {
	data, e := os.ReadFile(filepath.Join(c.dir, c.archiveName()))
	if e != nil {
		return e
	}
	return os.WriteFile(destination, data, 0600)
}
func (c *jackApplyClient) FetchChecksumFile(_ context.Context, url string) ([]byte, error) {
	if strings.HasSuffix(url, "jack-release.json") {
		return json.Marshal(JackReleaseManifest{Format: 1, Version: c.version, UpstreamVersion: jackBase(c.version), RuntimeID: "helper-runtime", SchemaEpoch: 1})
	}
	data, e := os.ReadFile(filepath.Join(c.dir, c.archiveName()))
	if e != nil {
		return nil, e
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	if c.corrupt {
		sum = strings.Repeat("0", 64)
	}
	return []byte(sum + "  " + c.archiveName() + "\n"), nil
}
func writeJackTestArchive(t *testing.T, dir, version string, body []byte) {
	t.Helper()
	name := fmt.Sprintf("sub2api_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	f, e := os.Create(filepath.Join(dir, name))
	require.NoError(t, e)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "sub2api", Mode: 0755, Size: int64(len(body))}))
	_, e = tw.Write(body)
	require.NoError(t, e)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())
}
func TestJackAtomicUpdateAndRollback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux executable replacement")
	}
	executable, e := os.Executable()
	require.NoError(t, e)
	original, e := os.ReadFile(executable)
	require.NoError(t, e)
	for _, mode := range []string{"success", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			helper := filepath.Join(dir, "sub2api")
			require.NoError(t, os.WriteFile(helper, original, 0755))
			writeJackTestArchive(t, dir, "0.2.13-jack.2", []byte("#!/bin/sh\necho updated-jack\n"))
			writeJackTestArchive(t, dir, "0.2.13-jack.1", original)
			cmd := exec.Command(helper, "-test.run=^TestJackUpdateHelper$", "-test.v")
			cmd.Env = append(os.Environ(), "JACK_UPDATE_HELPER="+mode, "JACK_UPDATE_TEST_DIR="+dir)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
			after, err := os.ReadFile(helper)
			require.NoError(t, err)
			require.True(t, bytes.Equal(original, after), "original binary must survive corruption or be restored by rollback")
			_, err = os.Stat(filepath.Join(dir, "backed-up"))
			if mode == "corrupt" {
				require.True(t, os.IsNotExist(err))
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestJackUpdateHelper(t *testing.T) {
	mode := os.Getenv("JACK_UPDATE_HELPER")
	if mode == "" {
		return
	}
	dir := os.Getenv("JACK_UPDATE_TEST_DIR")
	JackRuntimeID = "helper-runtime"
	client := &jackApplyClient{version: "0.2.13-jack.2", dir: dir, corrupt: mode == "corrupt"}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.2.13-jack.1", "release")
	svc.beforeUpdate = func(context.Context) error {
		return os.WriteFile(filepath.Join(dir, "backed-up"), []byte("complete"), 0600)
	}
	err := svc.PerformUpdate(context.Background())
	if mode == "corrupt" {
		require.ErrorContains(t, err, "checksum")
		return
	}
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "sub2api"))
	require.NoError(t, err)
	require.Contains(t, string(data), "updated-jack")
	svc.currentVersion = "0.2.13-jack.2"
	client.version = "0.2.13-jack.1"
	require.NoError(t, svc.RollbackToVersion(context.Background(), "0.2.13-jack.1"))
}
