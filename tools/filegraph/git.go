package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
)

type changedFile struct {
	Status byte
	Score  int
	Path   Path
	Path2  Path

	Added   int
	Deleted int
}

type commit struct {
	Hash  string
	Files []*changedFile
}

var errStop = fmt.Errorf("stop the iteration")

func ensureSameRepo(files ...string) (gitPath string, err error) {
	for _, f := range files {
		switch gp, err := absoluteGitPath(f); {
		case err != nil:
			return "", err

		case gitPath == "":
			gitPath = gp
		case gitPath != gp:
			return "", errors.Reason("%q and %q reside in different git repositories", files[0], f).Err()
		}
	}
	return gitPath, nil
}

// dirFromPath returns path as is if it is a dir, otherwise the dir containing
// the file at path.
func dirFromPath(path string) (dir string, err error) {
	switch stat, err := os.Stat(path); {
	case err != nil:
		return "", err
	case stat.IsDir():
		return path, nil
	default:
		return filepath.Dir(path), nil
	}
}

func gitRepoRoot(path string) (string, error) {
	return git(path, "rev-parse", "--show-toplevel")
}

func absoluteGitPath(path string) (string, error) {
	return git(path, "rev-parse", "--absolute-git-dir")
}

func git(path string, args ...string) (output string, err error) {
	dir, err := dirFromPath(path)
	if err != nil {
		return "", err
	}

	args = append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", args...)
	stdoutBytes, err := cmd.Output()
	output = strings.TrimSuffix(string(stdoutBytes), "\n")
	return output, errors.Annotate(err, "git %q failed", args).Err()
}

func readCommits(ctx context.Context, dir, exclude string, fn func(c commit) error) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	args := []string{
		"-C", dir,
		"log",
		"--format=format:%H",
		"--raw",
		"--numstat",
		"-z",
		"--reverse",
		"--first-parent",
	}
	if exclude != "" {
		args = append(args, exclude+"...")
	}
	cmd := exec.CommandContext(ctx, "git", args...)

	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		cancel()
		werr := cmd.Wait()
		if err == nil && werr != nil {
			err = errors.Reason("git log failed: %s. stderr: %s", werr, stderr).Err()
		}
	}()

	log := newLogReader(stdout)
	return log.ReadCommits(fn)
}
