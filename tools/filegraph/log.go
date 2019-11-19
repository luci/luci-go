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
	"bufio"
	"io"
	"strconv"

	"go.chromium.org/luci/common/errors"
)

// logParser parses output of git-log with options
//   -z --raw --numstat --format=format:%H
type logParser struct {
	r       *bufio.Reader
	hashBuf [40]byte

	// the delimiter character. NUL by default, customized in tests because NUL
	// is not allowed in string literals, so it is inconvenient to write test
	// cases with it.
	delim byte
}

func newLogParser(r io.Reader) *logParser {
	return &logParser{r: bufio.NewReader(r)}
}

// read reads a string until delim.
// The returned string does not contain the delim.
func (p *logParser) read(delim byte) (string, error) {
	ret, err := p.r.ReadString(delim)
	if err != nil {
		return "", err
	}

	ret = ret[:len(ret)-1]
	return ret, nil
}

// peek returns the next byte.
// Does not advance the reader position.
func (p *logParser) peek() (byte, error) {
	ret, err := p.r.ReadByte()
	if err == nil {
		err = p.r.UnreadByte()
	}
	return ret, err
}

// readRawBlock reads one file change in raw format.
// Doc: https://git-scm.com/docs/git-diff#_raw_output_format
func (p *logParser) readRawBlock(dest *fileChange) error {
	// Skip 4 sub-blocks separated by space.
	for i := 0; i < 4; i++ {
		if _, err := p.read(' '); err != nil {
			return err
		}
	}

	// Read status.
	status, err := p.read(p.delim)
	if err != nil {
		return err
	}
	dest.Status = fileStatus(status[0])
	if len(status) > 1 {
		if dest.Score, err = strconv.Atoi(status[1:]); err != nil {
			return errors.Annotate(err, "failed to parse score").Err()
		}
	}

	// Read src.
	path, err := p.read(p.delim)
	if err != nil {
		return err
	}
	dest.Src = ParsePath(path)

	// Read dst.
	if dest.Status == renamed || dest.Status == copied {
		path, err = p.read(p.delim)
		if err != nil {
			return err
		}
		dest.Dst = ParsePath(path)
	}

	return nil
}

// expect reads a byte and return a non-nil error if it is unexpected.
func (p *logParser) expect(expected byte) error {
	switch actual, err := p.r.ReadByte(); {
	case err != nil:
		return err
	case actual != expected:
		return errors.Reason("expected %d byte, got %d", expected, actual).Err()
	default:
		return nil
	}
}

// readNum reads an integer followed by \t.
func (p *logParser) readNum() (int, error) {
	s, err := p.read('\t')
	if err != nil {
		return 0, err
	}
	if s == "-" {
		return -1, nil
	}
	return strconv.Atoi(s)
}

// readNumStat reads one numstat row.
// Doc: https://git-scm.com/docs/git-diff#_other_diff_formats
func (p *logParser) readNumStat(dest *fileChange) (err error) {
	// Read number of added/deleted lines.
	if dest.AddedLines, err = p.readNum(); err != nil {
		return err
	}
	if dest.DeletedLines, err = p.readNum(); err != nil {
		return err
	}

	// If there are two paths, there must be a NUL here.
	if dest.Dst != nil {
		if err := p.expect(p.delim); err != nil {
			return err
		}
	}

	// Read and verify paths.
	verifyPath := func(expected Path) error {
		switch actual, err := p.read(p.delim); {
		case err != nil:
			return err
		case actual != expected.String():
			return errors.Reason("expected path %q; got %q", expected, actual).Err()
		default:
			return nil
		}
	}

	if err := verifyPath(dest.Src); err != nil {
		return err
	}

	if dest.Dst != nil {
		if err := verifyPath(dest.Dst); err != nil {
			return err
		}
	}

	return nil
}

// ReadCommit reads one git commit.
// It has 3 sections:
//  - 40-char commit hash
//  - a raw block, an entry per file
//  - a numstat block, an entry per file in the same order as the raw block
func (p *logParser) ReadCommit() (commit, error) {
	c := commit{}

	// If a commit is empty, it is not followed by \n.
	// Read exactly 40 characters.
	if _, err := io.ReadFull(p.r, p.hashBuf[:]); err != nil {
		return c, err
	}
	c.Hash = string(p.hashBuf[:])
	if !looksLikeHash(c.Hash) {
		return c, errors.Reason("expected a hash; got %q", c.Hash).Err()
	}

	// Skip delimiter.
	switch b, err := p.r.ReadByte(); {
	case err != nil:
		return c, err
	case b == p.delim:
		// This is an empty commit.
		return c, nil
	case b == '\n':
		// This is a non-empty commit.
	default:
		return c, errors.Reason("expected \\n or delimiter; got %d byte", b).Err()
	}

	// Read the raw block.
	for {
		next, err := p.peek()
		if err != nil {
			return c, err
		}
		if next != ':' {
			// The raw block has ended.
			break
		}

		file := &fileChange{}
		if err := p.readRawBlock(file); err != nil {
			return c, err
		}
		c.Files = append(c.Files, file)
	}

	// Now read num stats for each file we've read, in the same order.
	for _, f := range c.Files {
		if err := p.readNumStat(f); err != nil {
			return c, err
		}
	}

	return c, nil
}

// ReadCommits calls fn for each commit in the log.
func (p *logParser) ReadCommits(fn func(commit) error) error {
	for {
		c, err := p.ReadCommit()
		switch {
		case err == io.EOF:
			return nil

		case err != nil:
			return err
		}

		if err := fn(c); err != nil {
			if err == errStop {
				// We are asked to stop.
				err = nil
			}
			return err
		}

		// Read the next delimiter.
		switch b, err := p.r.ReadByte(); {
		case err == io.EOF:
			return nil // this was the last commit

		case err != nil:
			return err

		case b == p.delim:
			// This is expected.

		default:
			// Not a delimiter. Put it back.
			if err := p.r.UnreadByte(); err != nil {
				return err
			}
		}
	}
}

func looksLikeHash(s string) bool {
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
