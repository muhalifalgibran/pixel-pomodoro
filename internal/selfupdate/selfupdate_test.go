package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildArchive produces a release tarball holding the given binary contents.
func buildArchive(t *testing.T, tag, goos, goarch string, binary []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	dir := fmt.Sprintf("pomo_%s_%s_%s", tag, goos, goarch)
	for _, f := range []struct {
		name string
		body []byte
	}{
		{dir + "/README.md", []byte("readme")},
		{dir + "/pomo", binary},
	} {
		if err := tw.WriteHeader(&tar.Header{
			Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// fakeRelease serves a GitHub-shaped release plus its assets.
type fakeRelease struct {
	tag      string
	archive  []byte
	checksum string // overrides the real hash when non-empty
	omitSums bool
	server   *httptest.Server
}

func newFakeRelease(t *testing.T, f *fakeRelease) *fakeRelease {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.server = srv

	assetName := AssetName(f.tag, "linux", "amd64")

	sum := f.checksum
	if sum == "" {
		sum = sha256Hex(f.archive)
	}
	sums := fmt.Sprintf("%s  ./%s\n", sum, assetName)

	mux.HandleFunc("/repos/owner/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		rel := Release{TagName: f.tag}
		rel.Assets = append(rel.Assets, Asset{Name: assetName, URL: srv.URL + "/dl/" + assetName})
		if !f.omitSums {
			rel.Assets = append(rel.Assets, Asset{Name: "checksums.txt", URL: srv.URL + "/dl/checksums.txt"})
		}
		json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("/dl/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		w.Write(f.archive)
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sums))
	})
	return f
}

func (f *fakeRelease) options(t *testing.T, current, execPath string) Options {
	t.Helper()
	return Options{
		Repo:       "owner/repo",
		Current:    current,
		BaseURL:    f.server.URL,
		Out:        discard{},
		TargetOS:   "linux",
		TargetArch: "amd64",
		ExecPath:   execPath,
	}
}

// discard is a value-type writer, so Options.Out is non-nil in tests.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func installedAt(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pomo")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAssetName(t *testing.T) {
	tests := []struct {
		tag, goos, goarch, want string
	}{
		{"v0.1.2", "darwin", "arm64", "pomo_v0.1.2_darwin_arm64.tar.gz"},
		{"v0.1.2", "linux", "amd64", "pomo_v0.1.2_linux_amd64.tar.gz"},
		{"v0.1.2", "windows", "amd64", "pomo_v0.1.2_windows_amd64.zip"},
	}
	for _, tt := range tests {
		if got := AssetName(tt.tag, tt.goos, tt.goarch); got != tt.want {
			t.Errorf("AssetName(%q,%q,%q) = %q, want %q", tt.tag, tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestChecksumFor(t *testing.T) {
	listing := "aaa  ./pomo_v1_linux_amd64.tar.gz\nbbb  pomo_v1_darwin_arm64.tar.gz\nccc *pomo_v1_windows_amd64.zip\n"

	for _, tt := range []struct{ name, want string }{
		{"pomo_v1_linux_amd64.tar.gz", "aaa"},
		{"pomo_v1_darwin_arm64.tar.gz", "bbb"},
		{"pomo_v1_windows_amd64.zip", "ccc"},
	} {
		got, err := checksumFor(listing, tt.name)
		if err != nil {
			t.Errorf("checksumFor(%q) error = %v", tt.name, err)
			continue
		}
		if got != tt.want {
			t.Errorf("checksumFor(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}

	if _, err := checksumFor(listing, "absent.tar.gz"); err == nil {
		t.Error("checksumFor found a hash for a file that is not listed")
	}
}

func TestSameVersionIgnoresTheVPrefix(t *testing.T) {
	if !sameVersion("v0.1.1", "0.1.1") || !sameVersion("0.1.1", "v0.1.1") {
		t.Error("sameVersion should tolerate a missing v prefix")
	}
	if sameVersion("v0.1.1", "v0.1.2") {
		t.Error("different versions compared equal")
	}
}

func TestRunReplacesTheBinary(t *testing.T) {
	newBinary := []byte("the new pomo")
	f := newFakeRelease(t, &fakeRelease{tag: "v9.9.9", archive: buildArchive(t, "v9.9.9", "linux", "amd64", newBinary)})
	path := installedAt(t, "the old pomo")

	opts := f.options(t, "v0.1.0", path)
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newBinary) {
		t.Errorf("installed %q, want %q", got, newBinary)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("installed binary is not executable: %v", info.Mode())
	}
}

func TestRunReportsUpToDate(t *testing.T) {
	f := newFakeRelease(t, &fakeRelease{tag: "v0.1.1", archive: buildArchive(t, "v0.1.1", "linux", "amd64", []byte("x"))})
	path := installedAt(t, "current")

	err := Run(context.Background(), f.options(t, "v0.1.1", path))

	if !errors.Is(err, ErrUpToDate) {
		t.Errorf("Run() error = %v, want ErrUpToDate", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "current" {
		t.Error("an up-to-date binary was replaced anyway")
	}
}

// The whole point of the feature's safety story: a tampered download must
// never reach the filesystem as an executable.
func TestRunRefusesAChecksumMismatch(t *testing.T) {
	f := newFakeRelease(t, &fakeRelease{
		tag:      "v9.9.9",
		archive:  buildArchive(t, "v9.9.9", "linux", "amd64", []byte("malicious")),
		checksum: strings.Repeat("00", 32), // a hash the download cannot match
	})
	path := installedAt(t, "the old pomo")

	err := Run(context.Background(), f.options(t, "v0.1.0", path))

	if err == nil {
		t.Fatal("Run() installed a binary whose checksum did not match")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %v, want it to name the checksum mismatch", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "the old pomo" {
		t.Error("the existing binary was replaced despite the mismatch")
	}
	// Nothing should be left behind in the install directory either.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("install directory holds %d files, want just the original binary", len(entries))
	}
}

func TestRunRefusesAReleaseWithoutChecksums(t *testing.T) {
	f := newFakeRelease(t, &fakeRelease{
		tag:      "v9.9.9",
		archive:  buildArchive(t, "v9.9.9", "linux", "amd64", []byte("x")),
		omitSums: true,
	})
	path := installedAt(t, "the old pomo")

	err := Run(context.Background(), f.options(t, "v0.1.0", path))

	if err == nil || !strings.Contains(err.Error(), "checksums") {
		t.Errorf("Run() error = %v, want a refusal to install unverified", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "the old pomo" {
		t.Error("the binary was replaced without any checksum to verify against")
	}
}

func TestRunReportsAMissingPlatformBuild(t *testing.T) {
	f := newFakeRelease(t, &fakeRelease{tag: "v9.9.9", archive: buildArchive(t, "v9.9.9", "linux", "amd64", []byte("x"))})
	path := installedAt(t, "old")

	opts := f.options(t, "v0.1.0", path)
	opts.TargetOS, opts.TargetArch = "plan9", "mips"

	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "plan9/mips") {
		t.Errorf("Run() error = %v, want it to name the missing platform", err)
	}
}

func TestConfirmCanCancel(t *testing.T) {
	f := newFakeRelease(t, &fakeRelease{tag: "v9.9.9", archive: buildArchive(t, "v9.9.9", "linux", "amd64", []byte("new"))})
	path := installedAt(t, "old")

	opts := f.options(t, "v0.1.0", path)
	asked := false
	opts.Confirm = func(from, to string) bool {
		asked = true
		if from != "v0.1.0" || to != "v9.9.9" {
			t.Errorf("Confirm(%q, %q), want (v0.1.0, v9.9.9)", from, to)
		}
		return false
	}

	if err := Run(context.Background(), opts); err == nil {
		t.Error("Run() proceeded after Confirm returned false")
	}
	if !asked {
		t.Error("Confirm was never called")
	}
	if got, _ := os.ReadFile(path); string(got) != "old" {
		t.Error("the binary was replaced despite cancelling")
	}
}

func TestExtractBinaryRejectsAnArchiveWithoutPomo(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "dir/README.md", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	tw.Write([]byte("x"))
	tw.Close()
	gz.Close()

	if _, err := extractBinary(buf.Bytes(), "linux"); err == nil {
		t.Error("extractBinary accepted an archive with no pomo binary")
	}
}

func TestLatestSurfacesAnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Latest(context.Background(), Options{Repo: "owner/repo", BaseURL: srv.URL})
	if err == nil {
		t.Error("Latest() ignored a 404")
	}
}
