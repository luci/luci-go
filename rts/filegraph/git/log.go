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

package git

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os/exec"

	"go.chromium.org/luci/common/errors"
)

type commit struct {
	Hash  string
	Files []fileChange
}

type fileChange struct {
	// Status is the file change type: 'A', 'C', 'D', 'R', 'M', etc.
	// For more info, see --diff-filter in https://git-scm.com/docs/git-diff
	Status byte
	Path   string
	Path2  string // populate if Status is 'R'
}

// readLog calls the callback for each commit reachable from `rev` and not
// reachable from `exclude`.
// Returns only first-parents, using --first-parent git flag.
func readLog(ctx context.Context, repoDir, exclude, rev string, callback func(commit) error) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	args := []string{
		"-C", repoDir,
		"log",
		"--format=format:%H",
		"--raw",
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
	return log.ReadCommits(callback)
}

type logParser struct {
	r       *bufio.Reader
	hashBuf [40]byte
	err     error
	sep     byte
}

func newLogReader(r io.Reader) *logParser {
	return &logParser{r: bufio.NewReader(r)}
}

func (r *logParser) read(delim byte) (string, error) {
	ret, err := r.r.ReadString(delim)
	if err != nil {
		return "", err
	}

	ret = ret[:len(ret)-1]
	return ret, nil
}

func (r *logParser) peek() (byte, error) {
	ret, err := r.r.ReadByte()
	if err == nil {
		err = r.r.UnreadByte()
	}
	return ret, err
}

func (r *logParser) readRawBlock() (file fileChange, err error) {
	// Format doc: https://git-scm.com/docs/git-diff#_raw_output_format

	// Skip 4 sub-blocks separated by space.
	for i := 0; i < 4; i++ {
		if _, err = r.read(' '); err != nil {
			return
		}
	}

	// Read status
	var statusString string
	if statusString, err = r.read(r.sep); err != nil {
		return
	}
	// For renames, it might look like R90. We need only the first char.
	file.Status = statusString[0]

	if file.Path, err = r.read(r.sep); err != nil {
		return
	}

	// If status is rename or copy, then read the second path.
	if file.Status == 'R' || file.Status == 'C' {
		if file.Path2, err = r.read(r.sep); err != nil {
			return
		}
	}

	return
}

func (r *logParser) expect(expected byte) error {
	// Assert NUL
	switch actual, err := r.r.ReadByte(); {
	case err != nil:
		return err
	case actual != expected:
		return errors.Reason("expected %d byte, got %d", expected, actual).Err()
	default:
		return nil
	}
}

func (r *logParser) ReadCommit() (commit, error) {
	c := commit{}
	// If a commit is empty, it is not followed by \n.
	// Read exactly 40 characters.
	if _, err := io.ReadFull(r.r, r.hashBuf[:]); err != nil {
		return c, err
	}
	c.Hash = string(r.hashBuf[:])
	if !lookLikeHash(c.Hash) {
		return c, errors.Reason("expected a hash; got %q", c.Hash).Err()
	}

	// Skip delimeter
	switch b, err := r.r.ReadByte(); {
	case err != nil:
		return c, err
	case b == r.sep:
		// This is an empty commit
		return c, nil
	case b == '\n':
		// expected
	default:
		return c, errors.Reason("expected \\n or separator; got %d byte", b).Err()
	}

	for {
		next, err := r.peek()
		if err != nil {
			return c, err
		}
		if next != ':' {
			// The raw block has ended.
			break
		}

		file, err := r.readRawBlock()
		if err != nil {
			return c, err
		}
		c.Files = append(c.Files, file)
	}

	// // Now read num stats for each file we've read, in the same order.
	// for _, f := range c.Files {
	// 	if err := r.readNumStat(f); err != nil {
	// 		return c, err
	// 	}
	// }

	return c, nil
}

func (r *logParser) ReadCommits(fn func(commit) error) error {
	for {
		c, err := r.ReadCommit()
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		}

		if err := fn(c); err != nil {
			return err
		}

		switch b, err := r.r.ReadByte(); {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		case b != r.sep:
			if err := r.r.UnreadByte(); err != nil {
				return err
			}
		}
	}
}

func lookLikeHash(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			// yes
		case c >= 'a' && c <= 'f':
			// yes
		default:
			return false
		}
	}
	return true
}
