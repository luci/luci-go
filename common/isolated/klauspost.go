// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Abstraction layer for the zlib package to support klauspost/compress/zlib import.

// +build !go1.7

package isolated

import (
	"io"

	"github.com/klauspost/compress/zlib"
)

func newZlibReader(r io.Reader) (io.ReadCloser, error) {
	return zlib.NewReader(r)
}

func newZlibWriterLevel(w io.Writer, level int) (*zlib.Writer, error) {
	return zlib.NewWriterLevel(w, level)
}
