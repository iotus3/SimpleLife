package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/update"
)

// packageBytes is a stand-in for a release package: a zip holding the program.
func packageBytes(t *testing.T, program []byte) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "package.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	writer := zip.NewWriter(file)
	for _, name := range []string{"README.txt", programName} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("zip entry: %v", err)
		}
		body := []byte("notes")
		if name == programName {
			body = program
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return data
}

func assetFor(data []byte) update.Asset {
	sum := sha256.Sum256(data)
	return update.Asset{
		Name:   "simplelife-0.2.0-windows-amd64.zip",
		SHA256: hex.EncodeToString(sum[:]),
		Size:   int64(len(data)),
	}
}

// packageServer serves fixed bytes and remembers how often it was asked.
func packageServer(t *testing.T, data []byte) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestDownloadKeepsTheVerifiedPackage(t *testing.T) {
	data := packageBytes(t, []byte("MZ-new-build"))
	server, _ := packageServer(t, data)
	asset := assetFor(data)
	asset.URL = server.URL + "/package.zip"

	installer := Installer{Client: server.Client()}
	dir := t.TempDir()
	path, err := installer.download(context.Background(), asset, dir, nil)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(stored) != string(data) {
		t.Fatalf("stored %d bytes, want %d", len(stored), len(data))
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("the folder must hold the package alone, got %d entries", len(entries))
	}
}

func TestDownloadDiscardsAPackageThatDoesNotMatch(t *testing.T) {
	data := packageBytes(t, []byte("MZ-build"))
	server, _ := packageServer(t, data)
	asset := assetFor(data)
	asset.URL = server.URL + "/package.zip"
	asset.SHA256 = strings.Repeat("0", 64)

	dir := t.TempDir()
	_, err := Installer{Client: server.Client()}.download(context.Background(), asset, dir, nil)
	if !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("want invalid, got %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a rejected download must leave nothing, got %d entries", len(entries))
	}
}

func TestDownloadRefusesALongerPackage(t *testing.T) {
	data := packageBytes(t, []byte("MZ-build"))
	server, _ := packageServer(t, data)
	asset := assetFor(data)
	asset.URL = server.URL + "/package.zip"
	asset.Size = asset.Size - 10

	_, err := Installer{Client: server.Client()}.download(context.Background(), asset, t.TempDir(), nil)
	if !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("want invalid, got %v", err)
	}
}

func TestExtractProgramFindsTheExecutable(t *testing.T) {
	program := []byte("MZ-the-real-program")
	archive := filepath.Join(t.TempDir(), "package.zip")
	if err := os.WriteFile(archive, packageBytes(t, program), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	path, err := extractProgram(archive, t.TempDir())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(program) {
		t.Fatalf("extracted %q", got)
	}
}

func TestExtractProgramRefusesAnArchiveWithoutIt(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "package.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	writer := zip.NewWriter(file)
	if _, err := writer.Create("README.txt"); err != nil {
		t.Fatalf("zip: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := extractProgram(archive, t.TempDir()); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("want invalid, got %v", err)
	}
}

func TestReplaceProgramSwapsTheFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, programName)
	if err := os.WriteFile(target, []byte("old"), 0o700); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := filepath.Join(dir, "payload")
	if err := os.WriteFile(payload, []byte("new"), 0o700); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := replaceProgram(target, payload); err != nil {
		t.Fatalf("replace: %v", err)
	}
	current, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(current) != "new" {
		t.Fatalf("the program is %q, want the new build", current)
	}
	retired, err := os.ReadFile(target + retiredSuffix)
	if err != nil {
		t.Fatalf("the previous build must be kept until the next start: %v", err)
	}
	if string(retired) != "old" {
		t.Fatalf("the retired build is %q", retired)
	}
}

func TestReplaceProgramPutsTheOldBuildBack(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, programName)
	if err := os.WriteFile(target, []byte("old"), 0o700); err != nil {
		t.Fatalf("write: %v", err)
	}
	// A payload that is not there is a failed copy; the original must come back.
	err := replaceProgram(target, filepath.Join(dir, "missing"))
	if !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("want invalid, got %v", err)
	}
	current, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("the program must be back in place: %v", readErr)
	}
	if string(current) != "old" {
		t.Fatalf("the program is %q, want the original", current)
	}
}

func TestCleanupPreviousRemovesTheRetiredBuild(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, programName)
	if err := os.WriteFile(target+retiredSuffix, []byte("old"), 0o700); err != nil {
		t.Fatalf("write: %v", err)
	}
	CleanupPrevious(target)
	if _, err := os.Stat(target + retiredSuffix); !os.IsNotExist(err) {
		t.Fatalf("the retired build is still there: %v", err)
	}
	// Nothing to remove is not an error, and neither is an empty path.
	CleanupPrevious(target)
	CleanupPrevious("")
}

func TestParseChecksumsReadsNameAndHash(t *testing.T) {
	hash := strings.Repeat("ab", 32)
	text := "# comment\n" + strings.Repeat("cd", 32) + "  other.zip\n" + hash + " *simplelife-0.2.0-windows-amd64.zip\n"
	got, err := parseChecksums(text, "simplelife-0.2.0-windows-amd64.zip")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != hash {
		t.Fatalf("hash = %q, want %q", got, hash)
	}
	if _, err := parseChecksums(text, "absent.zip"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a missing name must be invalid, got %v", err)
	}
}

func TestInstallRefusesAPackageItCannotVerify(t *testing.T) {
	installer := Installer{}
	release := update.Release{Version: "0.2.0", Asset: update.Asset{Name: "package.zip", URL: "https://example.com/x.zip", Size: 10}}
	if err := installer.Install(context.Background(), release, nil); err == nil {
		t.Fatal("an asset without a checksum must not be installed")
	}
}
