// Copyright 2017 The LUCI Authors.
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

// Abstraction layer for the zlib package to support standard golang zlib import.

// +build go1.7

package isolated

import (
	"compress/zlib"
	"io"
)

func newZlibReader(r io.Reader) (io.ReadCloser, error) {
	return zlib.NewReader(r)
}

func newZlibWriterLevel(w io.Writer, level int) (*zlib.Writer, error) {
	return zlib.NewWriterLevel(w, level)
}
