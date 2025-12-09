// Copyright 2015 The LUCI Authors.
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

package coordinatorTest

import (
	"errors"
	"io"

	storage "cloud.google.com/go/storage"

	"go.chromium.org/luci/common/gcloud/gs"
)

// GSClient is a testing Google Storage client implementation.
type GSClient map[gs.Path][]byte

// Put sets the data at a given path.
func (c GSClient) Put(path gs.Path, d []byte) {
	c[path] = d
}

// Get retrieves the data at the specific path.
func (c GSClient) Get(path gs.Path) []byte {
	return c[path]
}

// Attrs implements gs.Client.
func (c GSClient) Attrs(path gs.Path) (*storage.ObjectAttrs, error) { return nil, nil }

// Objects implements gs.Client.
func (c GSClient) Objects(path gs.Path) ([]*storage.ObjectAttrs, error) { return nil, nil }

// FirstObject implements gs.Client.
func (c GSClient) FirstObject(bucket, pattern string) (*storage.ObjectAttrs, error) { return nil, nil }

// SignedURL implements gs.Client.
func (c GSClient) SignedURL(p gs.Path, opts *storage.SignedURLOptions) (string, error) {
	return "", nil
}

// Close implements gs.Client.
func (c GSClient) Close() error { return nil }

// NewWriter implements gs.Client.
func (c GSClient) NewWriter(gs.Path) (gs.Writer, error) {
	return nil, errors.New("not implemented")
}

// Rename implements gs.Client.
func (c GSClient) Rename(gs.Path, gs.Path) error { return errors.New("not implemented") }

// Delete implements gs.Client.
func (c GSClient) Delete(gs.Path) error { return errors.New("not implemented") }

// NewReader implements gs.Client.
func (c GSClient) NewReader(path gs.Path, offset int64, length int64) (io.ReadCloser, error) {
	if d, ok := c["error"]; ok {
		return nil, errors.New(string(d))
	}

	d, ok := c[path]
	if !ok {
		return nil, errors.New("does not exist")
	}

	// Determine the slice of data to return.
	if offset < 0 {
		offset = 0
	}
	end := int64(len(d))
	if length >= 0 {
		if v := offset + length; v < end {
			end = v
		}
	}
	d = d[offset:end]

	r := make([]byte, len(d))
	copy(r, d)
	gsr := testGSReader(r)
	return &gsr, nil
}

type testGSReader []byte

func (r *testGSReader) Read(d []byte) (int, error) {
	if len(*r) == 0 {
		return 0, io.EOF
	}

	amt := copy(d, *r)
	*r = (*r)[amt:]
	return amt, nil
}

func (r *testGSReader) Close() error { return nil }
