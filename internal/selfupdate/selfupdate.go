// Package selfupdate replaces the running binary with the latest published
// release.
//
// The downloaded archive is checked against the release's published SHA-256
// before anything is written to disk. An unverified binary is never extracted,
// never made executable, and never put anywhere it could be run: a self-updater
// that skips that step is a remote code execution vector wearing a convenience
// feature's clothes.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
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

// maxDownload bounds what will be read from the network. The binaries are
// around 1.5 MB; anything approaching this is not a pomo release.
const maxDownload = 64 << 20

// DefaultRepo is where releases are published.
const DefaultRepo = "muhalifalgibran/pixel-pomodoro"

// Release is the subset of the GitHub release payload that matters here.
type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Asset is one published file.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Find returns the asset with the given name.
func (r Release) Find(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// Options configures an update run.
type Options struct {
	Repo    string
	Current string
	// BaseURL overrides the GitHub API host, for tests.
	BaseURL string
	Client  *http.Client
	Out     io.Writer
	// Confirm is asked before anything is replaced. A nil Confirm proceeds,
	// which is what -y is for.
	Confirm func(from, to string) bool
	// TargetOS and TargetArch default to the running platform.
	TargetOS, TargetArch string
	// ExecPath is the binary to replace; empty means the running one.
	ExecPath string
}

func (o *Options) fill() {
	if o.Repo == "" {
		o.Repo = DefaultRepo
	}
	if o.BaseURL == "" {
		o.BaseURL = "https://api.github.com"
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 60 * time.Second}
	}
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.TargetOS == "" {
		o.TargetOS = runtime.GOOS
	}
	if o.TargetArch == "" {
		o.TargetArch = runtime.GOARCH
	}
}

// AssetName is the archive published for a platform at a given tag.
func AssetName(tag, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("pomo_%s_%s_%s%s", tag, goos, goarch, ext)
}

// ErrUpToDate reports that no update is needed.
var ErrUpToDate = fmt.Errorf("already on the latest release")

// Latest fetches the newest published release.
func Latest(ctx context.Context, opts Options) (Release, error) {
	opts.fill()
	url := fmt.Sprintf("%s/repos/%s/releases/latest", strings.TrimSuffix(opts.BaseURL, "/"), opts.Repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := opts.Client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("check for updates: %s returned %s", url, resp.Status)
	}

	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("decode release: %w", err)
	}
	if rel.TagName == "" {
		return Release{}, fmt.Errorf("release has no tag")
	}
	return rel, nil
}

// Run performs the update. It reports ErrUpToDate when the running version
// already matches the latest release.
func Run(ctx context.Context, opts Options) error {
	opts.fill()

	rel, err := Latest(ctx, opts)
	if err != nil {
		return err
	}
	if sameVersion(opts.Current, rel.TagName) {
		fmt.Fprintf(opts.Out, "pomo %s is the latest release\n", rel.TagName)
		return ErrUpToDate
	}

	assetName := AssetName(rel.TagName, opts.TargetOS, opts.TargetArch)
	asset, ok := rel.Find(assetName)
	if !ok {
		return fmt.Errorf("release %s has no build for %s/%s (looked for %s)",
			rel.TagName, opts.TargetOS, opts.TargetArch, assetName)
	}
	sums, ok := rel.Find("checksums.txt")
	if !ok {
		// Without published checksums there is nothing to verify against, and
		// installing an unverified binary is not a trade worth making.
		return fmt.Errorf("release %s publishes no checksums.txt; refusing to install unverified", rel.TagName)
	}

	execPath := opts.ExecPath
	if execPath == "" {
		execPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("locate the running binary: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
			execPath = resolved
		}
	}

	if opts.Confirm != nil && !opts.Confirm(opts.Current, rel.TagName) {
		return fmt.Errorf("cancelled")
	}

	fmt.Fprintf(opts.Out, "downloading %s\n", asset.Name)
	archive, err := download(ctx, opts, asset.URL)
	if err != nil {
		return err
	}
	sumsBody, err := download(ctx, opts, sums.URL)
	if err != nil {
		return err
	}

	want, err := checksumFor(string(sumsBody), asset.Name)
	if err != nil {
		return err
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s: the download does not match the published hash, refusing to install", asset.Name)
	}
	fmt.Fprintln(opts.Out, "checksum verified")

	binary, err := extractBinary(archive, opts.TargetOS)
	if err != nil {
		return err
	}

	if err := replaceExecutable(execPath, binary); err != nil {
		return err
	}

	fmt.Fprintf(opts.Out, "updated %s to %s\n", execPath, rel.TagName)
	return nil
}

// sameVersion compares tags tolerantly: a binary may report v0.1.1 or 0.1.1
// depending on how it was built.
func sameVersion(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

func download(ctx context.Context, opts Options, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := opts.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	return body, nil
}

// checksumFor pulls one file's hash out of a `sha256sum` style listing.
func checksumFor(listing, name string) (string, error) {
	sc := bufio.NewScanner(strings.NewReader(listing))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		// Entries are written as "./name" or "name" depending on how the
		// listing was generated.
		if filepath.Base(strings.TrimPrefix(fields[1], "*")) == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", name)
}

// extractBinary pulls the pomo executable out of the release archive.
func extractBinary(archive []byte, goos string) ([]byte, error) {
	want := "pomo"
	if goos == "windows" {
		want = "pomo.exe"
	}
	if goos == "windows" {
		return extractFromZip(archive, want)
	}
	return extractFromTarGz(archive, want)
}

func extractFromTarGz(archive []byte, want string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != want {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxDownload))
		if err != nil {
			return nil, fmt.Errorf("read %s from archive: %w", want, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("archive contains no %s", want)
}

func extractFromZip(archive []byte, want string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) != want {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s in archive: %w", want, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, maxDownload))
		if err != nil {
			return nil, fmt.Errorf("read %s from archive: %w", want, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("archive contains no %s", want)
}

// replaceExecutable swaps the new binary into place.
//
// The temporary file is created in the same directory as the target so the
// rename is atomic rather than a cross-device copy: at no point is there a
// half-written pomo on the path.
func replaceExecutable(path string, binary []byte) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".pomo-update-*")
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("cannot write to %s: %w\ntry: sudo pomo -update", dir, err)
		}
		return fmt.Errorf("create temporary file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return fmt.Errorf("write update: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close update: %w", err)
	}
	// Only now, after the checksum passed, does this become executable.
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("make the update executable: %w", err)
	}

	if runtime.GOOS == "windows" {
		// Windows will not rename over a running executable, so move the old
		// one aside first. It is cleaned up on the next run.
		_ = os.Remove(path + ".old")
		if err := os.Rename(path, path+".old"); err != nil {
			return fmt.Errorf("move the old binary aside: %w", err)
		}
	}

	if err := os.Rename(tmpName, path); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("cannot replace %s: %w\ntry: sudo pomo -update", path, err)
		}
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
