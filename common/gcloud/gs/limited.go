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

package gs

import (
	"io"
)

// LimitedClient wraps a base Client, allowing additional limits to be applied
// to its calls.
type LimitedClient struct {
	// Base is the base Client instance.
	Client

	// MaxReadBytes, if >0, is the maximum number of bytes that can be read at a
	// time. If more bytes are required, additional read calls will be made.
	MaxReadBytes int64
}

// NewReader implements Client.
func (lc *LimitedClient) NewReader(p Path, offset, length int64) (io.ReadCloser, error) {
	if lc.MaxReadBytes <= 0 || (length > 0 && length <= lc.MaxReadBytes) {
		return lc.Client.NewReader(p, offset, length)
	}

	lr := limitedReader{
		lc:           lc,
		path:         p,
		maxReadBytes: lc.MaxReadBytes,
		nextOffset:   offset,
		nextLength:   length,
	}
	if err := lr.nextReader(); err != nil {
		return nil, err
	}
	return &lr, nil
}

type limitedReader struct {
	lc *LimitedClient

	maxReadBytes int64

	cur      io.ReadCloser
	curBytes int64

	path       Path
	nextOffset int64
	nextLength int64
}

func (lr *limitedReader) nextReader() error {
	if lr.nextLength == 0 {
		return io.EOF
	}

	if lr.cur != nil {
		if err := lr.closeCur(); err != nil {
			return err
		}
	}

	length, offset := lr.nextLength, lr.nextOffset
	if length < 0 || length > lr.maxReadBytes {
		length = lr.maxReadBytes
	}

	lr.nextOffset += length
	if lr.nextLength >= 0 {
		lr.nextLength -= length
	}
	lr.curBytes = 0

	var err error
	lr.cur, err = lr.lc.Client.NewReader(lr.path, offset, length)
	return err
}

func (lr *limitedReader) Read(b []byte) (total int, err error) {
	if lr.cur == nil {
		return 0, io.EOF
	}

	for len(b) > 0 {
		var amt int
		amt, err = lr.cur.Read(b)
		total += amt
		lr.curBytes += int64(amt)
		b = b[amt:]

		if err != io.EOF {
			break
		}

		if lr.nextLength < 0 && lr.curBytes < lr.maxReadBytes {
			// No more data in this reader, and we didn't read the full range. Mark
			// us as permanently EOF.
			lr.nextLength = 0
		}
		if err := lr.nextReader(); err != nil {
			return total, err
		}
	}
	return
}

func (lr *limitedReader) Close() (err error) { return lr.closeCur() }

func (lr *limitedReader) closeCur() error {
	if lr.cur == nil {
		return nil
	}

	closeMe := lr.cur
	lr.cur = nil
	return closeMe.Close()
}
