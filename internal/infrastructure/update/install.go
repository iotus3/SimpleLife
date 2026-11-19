package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/update"
)

// Installer downloads the package a release names and swaps it in for the running
// program. It does not start the new build: that is the composition root's job, because
// the single-instance claim has to be released first.
type Installer struct {
	// Client is the HTTP client; nil means no overall timeout, because a large package on
	// a slow link may take its time and the caller's context ends it.
	Client *http.Client
	// Exe overrides the program being replaced; empty means the running one.
	Exe string
}

// programName is the executable inside the package.
const programName = "simplelife.exe"

// retiredSuffix marks the previous build while the new one takes its place. Windows
// permits renaming a running image but not deleting it, so the old file is renamed and
// removed by the next start.
const retiredSuffix = ".old"

// Install implements the application's installer port.
func (i Installer) Install(ctx context.Context, release update.Release, progress func(update.Progress)) error {
	if runtime.GOOS != "windows" {
		return shared.Unsupported("update.install_unsupported",
			"only the Windows build replaces itself")
	}
	asset := release.Asset
	if !asset.Usable() {
		return shared.Invalid("update.asset", "asset", "the release names no usable package")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dir, err := os.MkdirTemp("", "simplelife-update-")
	if err != nil {
		return shared.Invalid("update.download", "", "could not prepare a download folder").Wrap(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	archive, err := i.download(ctx, asset, dir, progress)
	if err != nil {
		return err
	}
	payload, err := extractProgram(archive, dir)
	if err != nil {
		return err
	}
	report(progress, update.Progress{Stage: update.StageInstall, Done: 1, Size: 1})
	if err := replaceProgram(i.program(), payload); err != nil {
		return err
	}
	report(progress, update.Progress{Stage: update.StageRestart, Done: 1, Size: 1})
	return nil
}

// program is the file to replace, resolved so a shortcut that starts the program through
// a symbolic link still finds the image that is really running.
func (i Installer) program() string {
	if i.Exe != "" {
		return i.Exe
	}
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// download fetches the package into dir and returns its path. The file appears under its
// own name only once its length and digest are the release's: an interrupted, longer or
// altered download leaves nothing behind.
func (i Installer) download(ctx context.Context, asset update.Asset, dir string, progress func(update.Progress)) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", shared.Invalid("update.download", "", "unusable package address").Wrap(err)
	}
	response, err := i.client().Do(request)
	if err != nil {
		return "", shared.Invalid("update.unreachable", "", "could not reach the package").Wrap(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", shared.Invalid("update.status", "", "the package host answered "+response.Status)
	}

	final := filepath.Join(dir, asset.Name)
	part := final + ".part"
	file, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", shared.Invalid("update.download", "", "could not write the package").Wrap(err)
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(part)
		}
	}()

	sum := sha256.New()
	buffer := make([]byte, 64<<10)
	var done int64
	last := time.Now()
	for {
		// One byte past the promise tells a longer body from an exact one.
		count, readErr := response.Body.Read(buffer[:min(int64(len(buffer)), asset.Size+1-done)])
		if count > 0 {
			_, _ = sum.Write(buffer[:count])
			if _, err := file.Write(buffer[:count]); err != nil {
				return "", shared.Invalid("update.download", "", "could not write the package").Wrap(err)
			}
			done += int64(count)
			if done > asset.Size {
				return "", shared.Invalid("update.checksum", "", "the package is longer than the release says")
			}
			if progress != nil && time.Since(last) >= 100*time.Millisecond {
				last = time.Now()
				report(progress, update.Progress{Stage: update.StageDownload, Done: done, Size: asset.Size})
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", shared.Invalid("update.download", "", "the download ended early").Wrap(readErr)
		}
	}
	if done != asset.Size {
		return "", shared.Invalid("update.checksum", "", "the package is shorter than the release says")
	}
	if hex.EncodeToString(sum.Sum(nil)) != asset.SHA256 {
		return "", shared.Invalid("update.checksum", "", "the package does not match its checksum")
	}
	report(progress, update.Progress{Stage: update.StageDownload, Done: done, Size: asset.Size})
	if err := file.Close(); err != nil {
		return "", shared.Invalid("update.download", "", "could not close the package").Wrap(err)
	}
	if err := os.Rename(part, final); err != nil {
		return "", shared.Invalid("update.download", "", "could not keep the package").Wrap(err)
	}
	committed = true
	return final, nil
}

func (i Installer) client() *http.Client {
	if i.Client != nil {
		return i.Client
	}
	return &http.Client{}
}

// extractProgram writes the program out of the package and refuses an archive that does
// not carry one.
func extractProgram(archive, dir string) (string, error) {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return "", shared.Invalid("update.package", "", "the package is not a zip file").Wrap(err)
	}
	defer func() { _ = reader.Close() }()
	target := filepath.Join(dir, programName)
	for _, entry := range reader.File {
		if !strings.EqualFold(filepath.Base(entry.Name), programName) || entry.FileInfo().IsDir() {
			continue
		}
		source, err := entry.Open()
		if err != nil {
			return "", shared.Invalid("update.package", "", "the package entry is unreadable").Wrap(err)
		}
		defer func() { _ = source.Close() }()
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o700)
		if err != nil {
			return "", shared.Invalid("update.package", "", "could not unpack the program").Wrap(err)
		}
		written, copyErr := io.Copy(file, source)
		closeErr := file.Close()
		if copyErr != nil {
			return "", shared.Invalid("update.package", "", "could not unpack the program").Wrap(copyErr)
		}
		if closeErr != nil {
			return "", shared.Invalid("update.package", "", "could not unpack the program").Wrap(closeErr)
		}
		if written == 0 {
			return "", shared.Invalid("update.package", "", "the package holds an empty program")
		}
		return target, nil
	}
	return "", shared.Invalid("update.package", "", "the package holds no "+programName)
}

// replaceProgram puts payload in place of the program on disk. The running image is
// renamed out of the way first (Windows allows that, and not deleting it), and put back if
// the new one cannot be copied: a failed update always leaves a working program behind.
func replaceProgram(target, payload string) error {
	if target == "" || payload == "" {
		return shared.Invalid("update.replace", "", "no program to replace")
	}
	retired := target + retiredSuffix
	_ = os.Remove(retired)
	if err := os.Rename(target, retired); err != nil {
		return shared.Invalid("update.replace", "",
			"could not move the running program aside; close other copies and try again").Wrap(err)
	}
	if err := copyFile(payload, target); err != nil {
		if restoreErr := os.Rename(retired, target); restoreErr != nil {
			// The only copy left is retired; say so instead of reporting success.
			return shared.Invalid("update.replace", "",
				"the new program could not be written and the old one is at "+retired).Wrap(err)
		}
		return shared.Invalid("update.replace", "", "could not write the new program").Wrap(err)
	}
	return nil
}

// CleanupPrevious deletes the build a finished update left behind. It runs at start, when
// the previous process is gone and the file can finally be removed; a file still held by
// another copy is left for the next start.
func CleanupPrevious(program string) {
	if program == "" {
		return
	}
	if _, err := os.Stat(program + retiredSuffix); err != nil {
		return
	}
	// The previous process may be a moment from exiting; a short wait is cheaper than
	// leaving a stray executable beside the program.
	for attempt := 0; attempt < 5; attempt++ {
		if err := os.Remove(program + retiredSuffix); err == nil || errors.Is(err, os.ErrNotExist) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// report hands one progress step to the interface, which may not be listening.
func report(progress func(update.Progress), step update.Progress) {
	if progress != nil {
		progress(step)
	}
}
