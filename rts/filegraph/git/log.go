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
	Hash         string
	ParentHashes []string
	Files        []fileChange
}

type fileChange struct {
	// Status is the file change type: 'A', 'C', 'D', 'R', 'M', etc.
	// For more info, see --diff-filter in https://git-scm.com/docs/git-diff
	Status byte
	Path   string
	Path2  string // populated if Status is 'R'
}

// readLog calls the callback for each commit reachable from `rev` and not
// reachable from `exclude`. The order of commits is "reversed", i.e. ancestors
// first.
func readLog(ctx context.Context, repoDir, exclude, rev string, callback func(commit) error) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Run git-log.
	revRange := rev
	if exclude != "" {
		revRange = exclude + ".." + rev
	}
	cmd := exec.CommandContext(ctx,
		"git",
		"-C", repoDir,
		"log",
		"--format=format:%H %P",
		"--raw",
		"-z",
		"--reverse",
		revRange,
	)

	// Setup stdout and stderr.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return errors.Annotate(err, "failed to start git-log").Err()
	}
	defer func() {
		cancel()
		werr := cmd.Wait()
		if err == nil && werr != nil {
			err = errors.Reason("git log failed: %s\nstderr: %s", werr, stderr).Err()
		}
	}()

	parser := &logParser{r: bufio.NewReader(stdout)}
	return parser.ReadCommits(callback)
}

// logParser parses git log formatted as
//   --raw --z --format=format:"%H %P".
type logParser struct {
	r       *bufio.Reader
	hashBuf [40]byte
	sep     byte
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

	// Read the commit hash.
	var err error
	if c.Hash, err = r.readHash(); err != nil {
		return c, err
	}

	// Skip the space between %H and %P.
	if err := r.expect(' '); err != nil {
		return c, err
	}

	// Read parents, if any.
loop:
	for {
		switch b, err := r.peek(); {
		case err != nil:
			return c, err

		case b == r.sep || b == '\n':
			break loop

		default:
			parent, err := r.readHash()
			if err != nil {
				return c, err
			}
			c.ParentHashes = append(c.ParentHashes, parent)
		}
	}

	// Skip the delimiter.
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

	// Read the modified files.
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

// readHash reads exactly 40 characters as hash.
// Returns a non-nil error if it does not look like a hex hash.
func (r *logParser) readHash() (string, error) {
	if _, err := io.ReadFull(r.r, r.hashBuf[:]); err != nil {
		return "", err
	}
	ret := string(r.hashBuf[:])
	if !lookLikeHash(ret) {
		return "", errors.Reason("expected a hash; got %q", ret).Err()
	}
	return ret, nil
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
