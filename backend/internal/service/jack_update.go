package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// JackRuntimeID fingerprints image dependencies/resources at build time. A binary
// update is offered only when the installed runtime can execute that release.
var JackRuntimeID string

type JackReleaseManifest struct {
	Format          int    `json:"format"`
	Version         string `json:"version"`
	UpstreamVersion string `json:"upstream_version"`
	UpstreamCommit  string `json:"upstream_commit"`
	RuntimeID       string `json:"runtime_id"`
	SchemaEpoch     int    `json:"schema_epoch"`
}

func jackRevision(version string) int {
	_, suffix, ok := strings.Cut(version, "-jack.")
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(suffix)
	if err != nil || n < 1 {
		return 0
	}
	return n
}
func jackBase(version string) string {
	base, _, _ := strings.Cut(strings.TrimPrefix(version, "v"), "-jack.")
	return base
}
func validateJackAsset(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/"+githubRepo+"/releases/download/v") || strings.Contains(u.Path, "..") {
		return fmt.Errorf("asset is not a release from %s", githubRepo)
	}
	return nil
}
func (s *UpdateService) jackManifest(ctx context.Context, assets []Asset) (*JackReleaseManifest, error) {
	var manifestURL string
	for _, a := range assets {
		if a.Name == "jack-release.json" {
			manifestURL = a.DownloadURL
			break
		}
	}
	if manifestURL == "" {
		return nil, fmt.Errorf("Jack release manifest is missing")
	}
	if err := validateJackAsset(manifestURL); err != nil {
		return nil, err
	}
	data, err := s.githubClient.FetchChecksumFile(ctx, manifestURL)
	if err != nil {
		return nil, err
	}
	if len(data) > 65536 {
		return nil, fmt.Errorf("Jack release manifest is too large")
	}
	var m JackReleaseManifest
	if err = json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Format != 1 || m.SchemaEpoch != 1 || jackRevision(m.Version) == 0 || m.UpstreamVersion != jackBase(m.Version) || m.RuntimeID == "" {
		return nil, fmt.Errorf("unsupported Jack release manifest")
	}
	return &m, nil
}
func (s *UpdateService) checkJackRuntime(ctx context.Context, assets []Asset) (*JackReleaseManifest, error) {
	m, err := s.jackManifest(ctx, assets)
	if err != nil {
		return nil, err
	}
	if JackRuntimeID == "" || m.RuntimeID != JackRuntimeID {
		return nil, fmt.Errorf("this release requires a Docker image update; use deploy/jack/switch-image.sh with the selected Jack version")
	}
	return m, nil
}
