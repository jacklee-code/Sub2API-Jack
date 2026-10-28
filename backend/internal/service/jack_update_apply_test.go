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

	"github.com/Wei-Shaw/sub2api/resources"
	"github.com/stretchr/testify/require"
)

type jackApplyClient struct {
	version, dir string
	runtimeID    string
	corrupt      bool
}

func (c *jackApplyClient) runtime() string {
	if c.runtimeID != "" {
		return c.runtimeID
	}
	return "helper-runtime"
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
		return json.Marshal(JackReleaseManifest{Format: 1, Version: c.version, UpstreamVersion: jackBase(c.version), RuntimeID: c.runtime(), SchemaEpoch: 1})
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
	bundledBinary := buildJackPricingProbe(t)
	for _, mode := range []string{"success", "bundle", "corrupt", "backup-failed", "runtime-mismatch"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			helper := filepath.Join(dir, "sub2api")
			require.NoError(t, os.WriteFile(helper, original, 0755))
			updated := []byte("#!/bin/sh\necho updated-jack\n")
			if mode == "bundle" {
				updated = bundledBinary
			}
			writeJackTestArchive(t, dir, "0.2.13-jack.2", updated)
			writeJackTestArchive(t, dir, "0.2.13-jack.1", original)
			cmd := exec.Command(helper, "-test.run=^TestJackUpdateHelper$", "-test.v")
			cmd.Env = append(os.Environ(), "JACK_UPDATE_HELPER="+mode, "JACK_UPDATE_TEST_DIR="+dir)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
			after, err := os.ReadFile(helper)
			require.NoError(t, err)
			require.True(t, bytes.Equal(original, after), "original binary must survive corruption or be restored by rollback")
			_, err = os.Stat(filepath.Join(dir, "backed-up"))
			if mode == "corrupt" || mode == "backup-failed" || mode == "runtime-mismatch" {
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
	if mode == "runtime-mismatch" {
		client.runtimeID = "different-image"
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.2.13-jack.1", "release")
	svc.beforeUpdate = func(context.Context) error {
		if mode == "backup-failed" {
			return fmt.Errorf("fixture backup failed")
		}
		return os.WriteFile(filepath.Join(dir, "backed-up"), []byte("complete"), 0600)
	}
	err := svc.PerformUpdate(context.Background())
	if mode == "corrupt" {
		require.ErrorContains(t, err, "checksum")
		return
	}
	if mode == "runtime-mismatch" {
		require.ErrorContains(t, err, "Docker image update")
		return
	}
	if mode == "backup-failed" {
		require.ErrorContains(t, err, "pre-update backup failed")
		return
	}
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "sub2api"))
	require.NoError(t, err)
	if mode == "bundle" {
		output, e := exec.Command(filepath.Join(dir, "sub2api")).CombinedOutput()
		require.NoError(t, e, string(output))
		require.Contains(t, string(output), fmt.Sprintf("bundle-sha=%x", sha256.Sum256([]byte(jackPricingProbeData))))
	} else {
		require.Contains(t, string(data), "updated-jack")
	}
	svc.currentVersion = "0.2.13-jack.2"
	client.version = "0.2.13-jack.1"
	require.NoError(t, svc.RollbackToVersion(context.Background(), "0.2.13-jack.1"))
	if mode == "bundle" {
		probe := exec.Command(filepath.Join(dir, "sub2api"), "-test.run=^TestJackEmbeddedPricingProbe$")
		probe.Env = append(os.Environ(), "JACK_PRICING_PROBE=1")
		output, e := probe.CombinedOutput()
		require.NoError(t, e, string(output))
		require.Contains(t, string(output), fmt.Sprintf("bundle-sha=%x", sha256.Sum256([]byte(resources.ModelPricing()))))
	}
}

const jackPricingProbeData = `{"bundle-probe":{"input_cost_per_token":0.01,"output_cost_per_token":0.02}}`

// Build a second real executable using the production resource package with a
// different catalog, then exercise the updater and rollback with both binaries.
func buildJackPricingProbe(t *testing.T) []byte {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "resources/model-pricing"), 0755))
	embedSource, err := os.ReadFile("../../resources/jack_embedded.go")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "resources/jack_embedded.go"), embedSource, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "resources/model-pricing/model_prices_and_context_window.json"), []byte(jackPricingProbeData), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/Wei-Shaw/sub2api\n\ngo 1.22\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte(`package main
import ("crypto/sha256"; "fmt"; "github.com/Wei-Shaw/sub2api/resources")
func main() { fmt.Printf("bundle-sha=%x\n", sha256.Sum256([]byte(resources.ModelPricing()))) }
`), 0644))
	build := exec.Command("go", "build", "-o", "probe", ".")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off")
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	binary, err := os.ReadFile(filepath.Join(root, "probe"))
	require.NoError(t, err)
	return binary
}

func TestJackEmbeddedPricingProbe(t *testing.T) {
	if os.Getenv("JACK_PRICING_PROBE") != "1" {
		return
	}
	fmt.Printf("bundle-sha=%x\n", sha256.Sum256([]byte(resources.ModelPricing())))
}
