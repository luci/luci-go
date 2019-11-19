// Copyright 2019 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	typeChanged   fileStatus = 'T'
	unmerged      fileStatus = 'U'
	unknownChange fileStatus = 'X'
)

// fileChange is one file change in a git commit.
// Doc: https://git-scm.com/docs/git-diff#Documentation/git-diff.txt-git-diff-filesltpatterngt82308203
type fileChange struct {
	Status fileStatus

	// Meaning depends on Status.
	//   - copied and renamed: percentage of similarity between the source and
	//     target of the move or copy
	//   - modified: the percentage of dissimilarity
	Score int

	// Path to the "src" file.
	Src Path

	// Path to the "dst" file, i.e. the new path of the file.
	Dst Path

	// Number of added lines.
	// For binary files, it is -1.
	AddedLines int

	// Number of deleted lines.
	// For binary files, it is -1.
	DeletedLines int
}

type commit struct {
	Hash  string
	Files []*fileChange
}

// errStop returned by a callback indicates that the iteration must stop.
var errStop = fmt.Errorf("stop the iteration")

// readCommits calls fn for each commit in the git repository in the order
// from parents to children, which is roughly the chrological order.
// If fn returns errStop, readCommits stops and returns nil.
//
// If exclude is non-empty, commits reachable from exclude are excluded.
// This is similar to "exclude.." revision range in git-log.
//
// It follows only first parents.
// TODO(nodir): follow all parents and include all parents in commit.
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
		args = append(args, exclude+"..")
	}
	cmd := exec.CommandContext(ctx, "git", args...)

	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return errors.Annotate(err, "git failed to start").Err()
	}
	defer func() {
		cancel()
		werr := cmd.Wait()
		if err == nil && werr != nil {
			err = errors.Reason("git log failed: %s. stderr: %s", werr, stderr).Err()
		}
	}()

	return newLogParser(stdout).ReadCommits(fn)
}
