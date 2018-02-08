// Copyright 2018 The LUCI Authors.
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

package gs

import (
	"io"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
)

// ReadAll reads a Google Storage file chunk by chunk and feeds it to 'w'.
//
// On success returns the generation number of the content it read.
func ReadAll(c context.Context, gs GoogleStorage, path string, bufSize int, w io.Writer) (gen int64, err error) {
	r, err := gs.Reader(c, path, 0)
	if err != nil {
		return 0, errors.Annotate(err, "failed to start reading google storage file").Err()
	}

	if int64(bufSize) > r.Size() {
		bufSize = int(r.Size())
	}
	buf := make([]byte, bufSize)

	n := 0
	offset := int64(0)
	for err == nil {
		if n, err = r.ReadAt(buf, offset); n != 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return 0, errors.Annotate(werr, "write failed").Err()
			}
		}
		offset += int64(n)
	}

	if err == io.EOF {
		return r.Generation(), nil
	}
	return 0, errors.Annotate(err, "read failed").Err()
}
