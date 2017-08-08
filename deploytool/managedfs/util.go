// Copyright 2016 The LUCI Authors.
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

package managedfs

import (
	"bytes"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"go.chromium.org/luci/common/errors"
)

func ensureDirectory(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return errors.Annotate(err, "failed to create directory [%s]", path).Err()
	}
	return nil
}

// createFile creates a new file at the target location and returns it for
// writing. This is built on to of "os.Create", as the latter will follow the
// file path if it is a symlink, and we want to actually delete the link and
// work with a new file.
func createFile(path string) (*os.File, error) {
	switch st, err := os.Lstat(path); {
	case isNotExist(err):
		// The path does not exist, so we're good.
		break

	case err != nil:
		return nil, errors.Annotate(err, "failed to lstat [%s]", path).Err()

	case st.IsDir():
		return nil, errors.Reason("cannot create; path [%s] is a directory", path).Err()

	default:
		// Exists, and is a file/link, so unlink.
		if err := os.Remove(path); err != nil {
			return nil, errors.Annotate(err, "failed to remove existing [%s]", path).Err()
		}
	}

	return os.Create(path)
}

func isValidSinglePathComponent(elem string) bool {
	return (len(elem) > 0) && (strings.IndexRune(elem, os.PathSeparator) < 0)
}

func isSubpath(root, path string) bool {
	switch {
	case len(path) < len(root):
		return false
	case path[:len(root)] != root:
		return false
	case len(path) > len(root):
		// path == root + "..."; make sure "..." begins with a path separator.
		r, _ := utf8.DecodeRuneInString(path[len(root):])
		return r == os.PathSeparator
	default:
		// path == root
		return true
	}
}

func isNotExist(err error) bool {
	return os.IsNotExist(errors.Unwrap(err))
}

func byteCompare(a, b io.Reader, aBuf, bBuf []byte) (bool, error) {
	for done := false; !done; {
		ac, err := a.Read(aBuf)
		if err != nil {
			if err != io.EOF {
				return false, errors.Annotate(err, "failed to read").Err()
			}
			ac = 0
			done = true
		}

		bc, err := b.Read(bBuf)
		if err != nil {
			if err != io.EOF {
				return false, errors.Annotate(err, "failed to read").Err()
			}
			bc = 0
		}

		if !(ac == bc && bytes.Equal(aBuf[:ac], bBuf[:bc])) {
			return false, nil
		}
	}
	return true, nil
}
