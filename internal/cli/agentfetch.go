package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

// agentRepo is the GitHub repo that ships the fallbakit-agent binary after the
// monorepo split. The CLI no longer bundles it, so `runner up` fetches it from
// the latest release on demand.
const agentRepo = "fallbakit/tunnel"

// ensureAgentBinary returns a path to fallbakit-agent, downloading the latest
// release from agentRepo into the CLI's config bin dir when it isn't already
// present.
func ensureAgentBinary(ctx context.Context) (string, error) {
	if bin, err := findAgentBinary(); err == nil {
		return bin, nil
	}
	dir, err := ensureConfigHome()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(dir, "bin", "fallbakit-agent")
	printInfo("fallbakit-agent not found — downloading the latest release from %s…", bold(agentRepo))
	if err := downloadAgentBinary(ctx, dest); err != nil {
		return "", fmt.Errorf("%w\n\nInstall it manually from https://github.com/%s/releases, "+
			"or set FALLBAKIT_AGENT_BIN to an existing binary", err, agentRepo)
	}
	printSuccess("Installed agent to %s", dim(dest))
	return dest, nil
}

// agentAssetName is the GoReleaser archive name for this platform. version is the
// release tag without its leading "v" (e.g. "0.1.0").
func agentAssetName(version, goos, goarch string) string {
	return fmt.Sprintf("fallbakit-agent_%s_%s_%s.tar.gz", version, goos, goarch)
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func downloadAgentBinary(ctx context.Context, dest string) error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("no prebuilt agent for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	client := &http.Client{Timeout: 60 * time.Second}

	rel, err := latestRelease(ctx, client)
	if err != nil {
		return err
	}
	version := strings.TrimPrefix(rel.TagName, "v")
	wantArchive := agentAssetName(version, runtime.GOOS, runtime.GOARCH)
	archiveURL := assetURL(rel, wantArchive)
	sumsURL := assetURL(rel, "checksums.txt")
	if archiveURL == "" || sumsURL == "" {
		return fmt.Errorf("release %s has no asset %q", rel.TagName, wantArchive)
	}

	archive, err := download(ctx, client, archiveURL)
	if err != nil {
		return err
	}
	sums, err := download(ctx, client, sumsURL)
	if err != nil {
		return err
	}
	want := checksumFor(string(sums), wantArchive)
	if want == "" {
		return fmt.Errorf("checksums.txt has no entry for %s", wantArchive)
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s", wantArchive)
	}
	return extractAgent(archive, dest)
}

func latestRelease(ctx context.Context, client *http.Client) (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", agentRepo)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func assetURL(rel *ghRelease, name string) string {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a.URL
		}
	}
	return ""
}

func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// checksumFor returns the lowercase sha256 hex for name from a GoReleaser
// checksums.txt ("<sha256>  <filename>" per line).
func checksumFor(sums, name string) string {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return strings.ToLower(fields[0])
		}
	}
	return ""
}

// extractAgent pulls the fallbakit-agent entry out of a .tar.gz and writes it to
// dest (0755) atomically.
func extractAgent(targz []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(targz))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("archive has no fallbakit-agent binary")
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) != "fallbakit-agent" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		tmp := dest + ".tmp"
		out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil { //nolint:gosec // size bounded by release archive
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return os.Rename(tmp, dest)
	}
}
