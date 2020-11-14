// Copyright 2020 The LUCI Authors.
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

package filegraph

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"go.chromium.org/luci/common/errors"
)

type changedFile struct {
	Status byte
	Score  int
	Path   Path
	Path2  Path

	// Added   int
	// Deleted int
}

type commit struct {
	Hash  string
	Files []*changedFile
}

// ErrStop returned by Graph.Query's callback instructs Query to stop the iteration.
var ErrStop = fmt.Errorf("stop the iteration")

func readCommits(ctx context.Context, dir, exclude string, fn func(c commit) error) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	args := []string{
		"-C", dir,
		"log",
		"--format=format:%H",
		"--raw",
		//"--numstat",
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
