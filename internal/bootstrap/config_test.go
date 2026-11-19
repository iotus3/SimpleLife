package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPutsTheDataBesideTheProgram(t *testing.T) {
	base := t.TempDir()
	config, err := resolveConfig(func() (string, error) { return base, nil })
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if want := filepath.Join(base, "datas"); config.DataDir != want {
		t.Errorf("data directory is %q, want %q", config.DataDir, want)
	}
	if want := filepath.Join(base, "datas", "simplelife.db"); config.DBPath != want {
		t.Errorf("database path is %q, want %q", config.DBPath, want)
	}
	info, err := os.Stat(config.DataDir)
	if err != nil {
		t.Fatalf("the data directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("the data directory is not a directory")
	}
}

func TestResolveConfigFallsBackToTheWorkingDirectory(t *testing.T) {
	base := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	if err := os.Chdir(base); err != nil {
		t.Fatalf("change directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore directory: %v", err)
		}
	})

	config, err := resolveConfig(func() (string, error) { return "", errors.New("no program folder") })
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if filepath.Base(config.DataDir) != "datas" {
		t.Errorf("data directory is %q, want it to end in datas", config.DataDir)
	}
	if filepath.Dir(config.DBPath) != config.DataDir {
		t.Errorf("the database is at %q, outside %q", config.DBPath, config.DataDir)
	}
	if _, err := os.Stat(config.DataDir); err != nil {
		t.Fatalf("the fallback directory was not created: %v", err)
	}
}

func TestExecutableDirIsTheProgramsFolder(t *testing.T) {
	dir, err := executableDir()
	if err != nil {
		t.Fatalf("executable directory: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("executable directory %q is not absolute", dir)
	}
	// The test binary runs from the package's own folder, so the directory the
	// program sits in is a real one.
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("executable directory %q is not a directory: %v", dir, err)
	}
}

func TestLoadConfigReadsTheProgramsOwnFolder(t *testing.T) {
	config, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if filepath.Base(config.DataDir) != "datas" {
		t.Errorf("data directory is %q, want it to end in datas", config.DataDir)
	}
	if _, err := os.Stat(config.DBPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("opening the configuration created a database already: %v", err)
	}
	// The folder is created beside the test binary, which lives in the build
	// cache: leave nothing behind.
	t.Cleanup(func() { _ = os.RemoveAll(config.DataDir) })
}
