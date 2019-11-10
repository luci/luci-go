package main

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

func (r *logParser) readRawBlock(dest *fileChange) (err error) {
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
	dest.Status = fileStatus(statusString[0])
	if len(statusString) > 1 {
		if dest.Score, err = strconv.Atoi(statusString[1:]); err != nil {
			return errors.Annotate(err, "failed to parse score").Err()
		}
	}

	var path string
	if path, err = r.read(r.sep); err != nil {
		return
	}
	dest.Src = ParsePath(path)

	// If status is rename or copy, then read the second path.
	if dest.Status == renamed || dest.Status == copied {
		if path, err = r.read(r.sep); err != nil {
			return
		}
		dest.Dst = ParsePath(path)
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

func (r *logParser) readNumStat(dest *fileChange) (err error) {
	// Format doc: https://git-scm.com/docs/git-diff#_other_diff_formats

	// Read numbe of added/deleted lines.

	readAddedDeleted := func() (int, error) {
		s, err := r.read('\t')
		if err != nil {
			return 0, err
		}
		if s == "-" {
			return -1, nil
		}
		return strconv.Atoi(s)
	}

	if dest.Added, err = readAddedDeleted(); err != nil {
		return
	}
	if dest.Deleted, err = readAddedDeleted(); err != nil {
		return
	}

	// If there are two paths, there must be a NUL here.
	if dest.Dst != nil {
		if err := r.expect(r.sep); err != nil {
			return err
		}
	}

	// Read and verify paths.

	verifyPath := func(expected Path) error {
		switch actual, err := r.read(r.sep); {
		case err != nil:
			return err
		case actual != expected.String():
			return errors.Reason("expected path %q; got %q", expected, actual).Err()
		default:
			return nil
		}
	}

	if err = verifyPath(dest.Src); err != nil {
		return
	}

	if dest.Dst != nil {
		if err = verifyPath(dest.Dst); err != nil {
			return
		}
	}

	return nil
}

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
