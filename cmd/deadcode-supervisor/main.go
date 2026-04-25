package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/garaekz/go-supervisor/internal/supervisor"
)

const version = "0.1.5"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("deadcode-supervisor %s\n", version)
		return
	}

	cfg, err := configFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read stdin: %v\n", err)
		os.Exit(2)
	}

	stdout, stderr, code := supervisor.RunOnce(cfg, supervisor.NormalizeInputBytes(input))
	if stdout != "" {
		fmt.Fprint(os.Stdout, stdout)
	}
	if stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}

	os.Exit(code)
}

func configFromEnv() (supervisor.Config, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return supervisor.Config{}, err
	}

	workerScript := os.Getenv("DEADCODE_WORKER_SCRIPT")
	if workerScript == "" {
		workerScript, err = firstExistingPath(workerScriptCandidates(workingDir))
		if err != nil {
			return supervisor.Config{}, err
		}
	}

	bootstrapPath := os.Getenv("DEADCODE_WORKER_BOOTSTRAP")
	if bootstrapPath == "" {
		bootstrapPath, err = firstExistingPath([]string{
			filepath.Join(workingDir, "bootstrap", "app.php"),
		})
		if err != nil {
			return supervisor.Config{}, err
		}
	}

	phpBinary := os.Getenv("DEADCODE_PHP_BINARY")
	if phpBinary == "" {
		phpBinary = "php"
	}

	return supervisor.Config{
		PHPBinary:     phpBinary,
		WorkerScript:  workerScript,
		BootstrapPath: bootstrapPath,
		WorkingDir:    workingDir,
	}, nil
}

func workerScriptCandidates(workingDir string) []string {
	executablePath, _ := os.Executable()
	executableDir := filepath.Dir(executablePath)

	return []string{
		filepath.Join(workingDir, "vendor", "deadcode", "deadcode-laravel", "bin", "ox-runtime-worker.php"),
		filepath.Join(workingDir, "bin", "ox-runtime-worker.php"),
		filepath.Join(executableDir, "..", "..", "deadcode-laravel", "bin", "ox-runtime-worker.php"),
		filepath.Join(executableDir, "..", "deadcode-laravel", "bin", "ox-runtime-worker.php"),
	}
}

func firstExistingPath(candidates []string) (string, error) {
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}

		resolved := filepath.Clean(candidate)
		info, err := os.Stat(resolved)
		if err == nil && !info.IsDir() {
			return resolved, nil
		}
	}

	return "", fmt.Errorf("none of the configured paths exists: %v", candidates)
}
