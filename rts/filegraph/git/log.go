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
	args := []string{
		"-C", repoDir,
		"log",
		"--format=format:%H %P",
		"--raw",
		"-z",
		"--reverse",
		rev,
	}
	if exclude != "" {
		args = append(args, "^"+exclude)
	}
	cmd := exec.CommandContext(ctx, "git", args...)

	// Setup stdout and stderr.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	// Start the process, and do not return until it is exits.
	if err := cmd.Start(); err != nil {
		return errors.Annotate(err, "failed to start git-log").Err()
	}
	defer cmd.Wait()

	reader := &logReader{r: bufio.NewReader(stdout)}
	if err := reader.ReadCommits(callback); err != nil {
		return err
	}

	if err := cmd.Wait(); err != nil {
		return errors.Annotate(err, "git log failed: %s", stderr).Err()
	}
	return nil
}

// logReader parses a git log formatted as
//   --format=format:"%H %P" --raw --z
type logReader struct {
	r *bufio.Reader

	// hashBuf is used to read commit hash.
	hashBuf [40]byte

	// sep is the separator byte.
	// git-log with -z flag uses 0 as the separator.
	// Can be changed to something else for the convenience of testing.
	sep byte
}

// ReadCommits calls the callback for each commit read from the input,
// until the input is exhausted or the callback returns a non-nil error.
func (r *logReader) ReadCommits(callback func(commit) error) error {
	// Read commits until io.EOF.
	for {
		c, err := r.ReadCommit()
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		}

		if err := callback(c); err != nil {
			return err
		}

		// If the next byte is a separator, then there is another commit.
		switch nextFollows, err := r.readIf(r.sep); {
		case err == io.EOF:
			return nil
		case err != nil || !nextFollows:
			return err
		}
	}
}

// ReadCommit reads one commit.
func (r *logReader) ReadCommit() (commit, error) {
	c := commit{}

	// Read the commit hash.
	var err error
	if c.Hash, err = r.readHash(); err != nil {
		return c, err
	}

	// Read the unconditional space between %H and %P.
	switch b, err := r.r.ReadByte(); {
	case err != nil:
		return c, err
	case b != ' ':
		return c, errors.Reason("expected ' ', got %d", b).Err()
	}

	// Read the parent hashes, if any.
	switch b, err := r.r.Peek(1); {
	case err != nil:
		return c, err
	case b[0] == r.sep:
		// This is an orphan commit without modified files.
		return c, nil
	case b[0] == '\n':
		// This is an orphan commit with some modified files.
	default:
		// This commit has parents.
		if c.ParentHashes, err = r.readParentHashes(); err != nil {
			return c, errors.Annotate(err, "failed to read parent hashes").Err()
		}
	}

	// Read the modified files, if any.
	switch hasFileChanges, err := r.readIf('\n'); {
	case err != nil:
		return c, err
	case hasFileChanges:
		if c.Files, err = r.readFileChanges(); err != nil {
			return c, errors.Annotate(err, "failed to read file changes").Err()
		}
	}

	return c, nil
}

// readParentHashes reads all parent hashes.
func (r *logReader) readParentHashes() (hashes []string, err error) {
	// There is at least one parent and they are separated by space.
	for {
		parent, err := r.readHash()
		if err != nil {
			return hashes, err
		}
		hashes = append(hashes, parent)

		if nextFollows, err := r.readIf(' '); err != nil || !nextFollows {
			return hashes, err
		}
	}
}

// readFileChanges reads all file changes.
func (r *logReader) readFileChanges() (changes []fileChange, err error) {
	// Read the file changes. Each one starts with ":".
	for {
		if nextFollows, err := r.readIf(':'); err != nil || !nextFollows {
			return changes, err
		}

		fc, err := r.readFileChange()
		if err != nil {
			return nil, err
		}
		changes = append(changes, fc)
	}
}

// readFileChange reads one file change.
func (r *logReader) readFileChange() (fc fileChange, err error) {
	// Format doc: https://git-scm.com/docs/git-diff#_raw_output_format

	// Skip 4 sub-blocks, each one ending with space.
	for i := 0; i < 4; i++ {
		if _, err = r.readString(' '); err != nil {
			return
		}
	}

	// Read status.
	switch status, err := r.readString(r.sep); {
	case err != nil:
		return fc, err
	case len(status) == 0:
		return fc, errors.Reason("unexpectedly empty file status").Err()
	default:
		// For renames, it might look like R90. We need only the first char.
		fc.Status = status[0]
	}

	// Read the file paths.
	if fc.Path, err = r.readString(r.sep); err != nil {
		return
	}
	// If status is a rename or copy, then read the second path.
	if fc.Status == 'R' || fc.Status == 'C' {
		if fc.Path2, err = r.readString(r.sep); err != nil {
			return
		}
	}

	return
}

// readHash reads exactly 40 characters as hash.
// Returns a non-nil error if it does not look like a hex hash.
func (r *logReader) readHash() (string, error) {
	if _, err := io.ReadFull(r.r, r.hashBuf[:]); err != nil {
		return "", err
	}
	ret := string(r.hashBuf[:])
	if !lookLikeHash(ret) {
		return "", errors.Reason("expected a hash; got %q", ret).Err()
	}
	return ret, nil
}

// readString reads a string until the delimiter.
// The returned string does not include the delimiter.
func (r *logReader) readString(delim byte) (string, error) {
	ret, err := r.r.ReadString(delim)
	if err != nil {
		return "", err
	}

	ret = ret[:len(ret)-1]
	return ret, nil
}

// readif reads one byte and returns (true, nil) if the next byte is expected.
// If it is unexpected, returns (false, nil) without advancing the cursor.
func (r *logReader) readIf(expected byte) (match bool, err error) {
	switch actual, err := r.r.ReadByte(); {
	case err == io.EOF:
		// Not a match.
		return false, nil
	case err != nil:
		return false, err
	case actual == expected:
		return true, nil
	default:
		return false, r.r.UnreadByte()
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
