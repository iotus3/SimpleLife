package bootstrap

import (
	"os"
	"path/filepath"
)

// Config is what the process needs before anything else exists.
type Config struct {
	// DataDir holds the database and nothing else.
	DataDir string
	// DBPath is the SQLite file inside DataDir.
	DBPath string
}

// dataDirName is the folder beside the program that holds the data, so a copy of
// the folder is a complete backup of the application's state.
const dataDirName = "datas"

// dbFileName is the database inside it.
const dbFileName = "simplelife.db"

// loadConfig decides where the data lives: datas/ beside the executable, falling back to
// the working directory when that folder is not writable.
func loadConfig() (Config, error) {
	return resolveConfig(executableDir)
}

// resolveConfig does the work of loadConfig with the program's folder looked up
// through programDir, so a test can point it at a temporary directory.
func resolveConfig(programDir func() (string, error)) (Config, error) {
	dir, err := programDir()
	if err != nil || dir == "" {
		dir, err = os.Getwd()
		if err != nil {
			return Config{}, err
		}
	}
	dataDir := filepath.Join(dir, dataDirName)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		// A read-only program folder is not a reason to refuse to start: fall back
		// to the working directory, which is writable on the machines this can
		// happen on.
		dir, cwdErr := os.Getwd()
		if cwdErr != nil {
			return Config{}, err
		}
		dataDir = filepath.Join(dir, dataDirName)
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return Config{}, err
		}
	}
	return Config{DataDir: dataDir, DBPath: filepath.Join(dataDir, dbFileName)}, nil
}

// executableDir is the folder the program runs from.
func executableDir() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	// The path is resolved so a shortcut or a launcher that starts the program
	// through a symbolic link still finds the folder the program really sits in.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}
	return filepath.Dir(resolved), nil
}
