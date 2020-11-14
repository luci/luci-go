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
	"bufio"
	"io"
	"strconv"

	"go.chromium.org/luci/common/errors"
)

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

func (r *logParser) readRawBlock(dest *changedFile) (err error) {
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
	dest.Status = statusString[0]
	if len(statusString) > 1 {
		if dest.Score, err = strconv.Atoi(statusString[1:]); err != nil {
			return errors.Annotate(err, "failed to parse score").Err()
		}
	}

	var path string
	if path, err = r.read(r.sep); err != nil {
		return
	}
	dest.Path = ParsePath(path)

	// If status is rename or copy, then read the second path.
	if dest.Status == 'R' || dest.Status == 'C' {
		if path, err = r.read(r.sep); err != nil {
			return
		}
		dest.Path2 = ParsePath(path)
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

// func (r *logParser) readNumStat(dest *changedFile) (err error) {
// 	// Format doc: https://git-scm.com/docs/git-diff#_other_diff_formats

// 	// Read numbe of added/deleted lines.

// 	readAddedDeleted := func() (int, error) {
// 		s, err := r.read('\t')
// 		if err != nil {
// 			return 0, err
// 		}
// 		if s == "-" {
// 			return -1, nil
// 		}
// 		return strconv.Atoi(s)
// 	}

// 	if dest.Added, err = readAddedDeleted(); err != nil {
// 		return
// 	}
// 	if dest.Deleted, err = readAddedDeleted(); err != nil {
// 		return
// 	}

// 	// If there are two paths, there must be a NUL here.
// 	if dest.Path2 != nil {
// 		if err := r.expect(r.sep); err != nil {
// 			return err
// 		}
// 	}

// 	// Read and verify paths.

// 	verifyPath := func(expected Path) error {
// 		switch actual, err := r.read(r.sep); {
// 		case err != nil:
// 			return err
// 		case actual != expected.String():
// 			return errors.Reason("expected path %q; got %q", expected, actual).Err()
// 		default:
// 			return nil
// 		}
// 	}

// 	if err = verifyPath(dest.Path); err != nil {
// 		return
// 	}

// 	if dest.Path2 != nil {
// 		if err = verifyPath(dest.Path2); err != nil {
// 			return
// 		}
// 	}

// 	return nil
// }

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

		file := &changedFile{}
		if err := r.readRawBlock(file); err != nil {
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
			if err == ErrStop {
				err = nil
			}
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
