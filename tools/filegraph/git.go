package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"go.chromium.org/luci/common/errors"
)

// fileStatus is a type of file change in a git commit.
// Doc: https://git-scm.com/docs/git-diff#Documentation/git-diff.txt-git-diff-filesltpatterngt82308203
type fileStatus byte

const (
	added         fileStatus = 'A'
	deleted       fileStatus = 'D'
	modified      fileStatus = 'M'
	copied        fileStatus = 'C'
	renamed       fileStatus = 'R'
	typeChange    fileStatus = 'T'
	unmerged      fileStatus = 'U'
	unknownChange fileStatus = 'X'
)

// changeFile describes a file change.
// Doc: https://git-scm.com/docs/git-diff#Documentation/git-diff.txt-git-diff-filesltpatterngt82308203
type fileChange struct {
	Status fileStatus

	// Meaning depends on Status.
	// - copied and renamed: percentage of similarity between the source and
	//   target of the move or copy
	// - modified: the percentage of dissimilarity
	Score int

	// Path to the "src" file.
	Src Path

	// Path to the "dst" file.
	// For example, the new file path.
	Dst Path

	// Number of new lines.
	Added int

	// Number of deleted lines.
	Deleted int
}

type commit struct {
	Hash  string
	Files []*fileChange
}

var errStop = fmt.Errorf("stop the iteration")

// readCommits calls fn for each commit in the git repository.
//
// If exclude is non-empty, commits reachable from (parents of) exclude will be
// excluded.
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
