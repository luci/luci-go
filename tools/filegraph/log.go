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

	// the delimeter character. NUL by default, customized in tests.
	delim byte
}

func newLogReader(r io.Reader) *logParser {
	return &logParser{r: bufio.NewReader(r)}
}

// read reads a string until delim.
// The returned string does not contain the delim.
func (r *logParser) read(delim byte) (string, error) {
	ret, err := r.r.ReadString(delim)
	if err != nil {
		return "", err
	}

	ret = ret[:len(ret)-1]
	return ret, nil
}

// peek returns the next byte.
// Does not advance the reader position.
func (r *logParser) peek() (byte, error) {
	ret, err := r.r.ReadByte()
	if err == nil {
		err = r.r.UnreadByte()
	}
	return ret, err
}

// rawRawBlock reads one file change in raw format.
// Doc: https://git-scm.com/docs/git-diff#_raw_output_format
func (r *logParser) readRawBlock(dest *fileChange) error {
	// Skip 4 sub-blocks separated by space.
	for i := 0; i < 4; i++ {
		if _, err := r.read(' '); err != nil {
			return err
		}
	}

	// Read status.
	status, err := r.read(r.delim)
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
	path, err := r.read(r.delim)
	if err != nil {
		return err
	}
	dest.Src = ParsePath(path)

	// Read dst.
	if dest.Status == renamed || dest.Status == copied {
		path, err = r.read(r.delim)
		if err != nil {
			return err
		}
		dest.Dst = ParsePath(path)
	}

	return nil
}

// expect reads a byte and return a non-nil error if it is unexpected.
func (r *logParser) expect(expected byte) error {
	switch actual, err := r.r.ReadByte(); {
	case err != nil:
		return err
	case actual != expected:
		return errors.Reason("expected %d byte, got %d", expected, actual).Err()
	default:
		return nil
	}
}

// readNum reads an integer followed by \t.
func (r *logParser) readNum() (int, error) {
	s, err := r.read('\t')
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
func (r *logParser) readNumStat(dest *fileChange) (err error) {
	// Read number of added/deleted lines.
	if dest.Added, err = r.readNum(); err != nil {
		return err
	}
	if dest.Deleted, err = r.readNum(); err != nil {
		return err
	}

	// If there are two paths, there must be a NUL here.
	if dest.Dst != nil {
		if err := r.expect(r.delim); err != nil {
			return err
		}
	}

	// Read and verify paths.
	verifyPath := func(expected Path) error {
		switch actual, err := r.read(r.delim); {
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
func (r *logParser) ReadCommit() (commit, error) {
	c := commit{}

	// If a commit is empty, it is not followed by \n.
	// Read exactly 40 characters.
	if _, err := io.ReadFull(r.r, r.hashBuf[:]); err != nil {
		return c, err
	}
	c.Hash = string(r.hashBuf[:])
	if !looksLikeHash(c.Hash) {
		return c, errors.Reason("expected a hash; got %q", c.Hash).Err()
	}

	// Skip delimiter.
	switch b, err := r.r.ReadByte(); {
	case err != nil:
		return c, err
	case b == r.delim:
		// This is an empty commit.
		return c, nil
	case b == '\n':
		// This is a non-empty commit.
	default:
		return c, errors.Reason("expected \\n or delimeter; got %d byte", b).Err()
	}

	// Read the raw block.
	for {
		next, err := r.peek()
		if err != nil {
			return c, err
		}
		if next != ':' {
			// The raw block has ended.
			break
		}

		file := &fileChange{}
		if err := r.readRawBlock(file); err != nil {
			return c, err
		}
		c.Files = append(c.Files, file)
	}

	// Now read num stats for each file we've read, in the same order.
	for _, f := range c.Files {
		if err := r.readNumStat(f); err != nil {
			return c, err
		}
	}

	return c, nil
}

// ReadCommits calls fn for each commit in the log.
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
			if err == errStop {
				// We are asked to stop.
				err = nil
			}
			return err
		}

		// Read the next delimiter.
		switch b, err := r.r.ReadByte(); {
		case err == io.EOF:
			return nil // this was the last commit

		case err != nil:
			return err

		case b == r.delim:
			// This is expected.

		default:
			// Not a delimeter. Put it back.
			if err := r.r.UnreadByte(); err != nil {
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
