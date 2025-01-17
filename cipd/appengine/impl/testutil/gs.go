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

package testutil

import (
	"bytes"
	"context"
	"io"

	"go.chromium.org/luci/cipd/appengine/impl/gs"
)

// NoopGoogleStorage implements gs.GoogleStorage interface by returning errors.
//
// Can be embedded into mock implementations that override some subset of
// methods.
type NoopGoogleStorage struct {
	Err error // an error to return or nil to panic
}

var _ gs.GoogleStorage = NoopGoogleStorage{}

// Size is part of gs.GoogleStorage interface.
func (n NoopGoogleStorage) Size(ctx context.Context, path string) (size uint64, exists bool, err error) {
	if n.Err == nil {
		panic("must not be called")
	}
	return 0, false, n.Err
}

// Exists is part of gs.GoogleStorage interface.
func (n NoopGoogleStorage) Exists(ctx context.Context, path string) (exists bool, err error) {
	if n.Err == nil {
		panic("must not be called")
	}
	return false, n.Err
}

// Copy is part of gs.GoogleStorage interface.
func (n NoopGoogleStorage) Copy(ctx context.Context, dst string, dstGen int64, src string, srcGen int64) error {
	if n.Err == nil {
		panic("must not be called")
	}
	return n.Err
}

// Delete is part of gs.GoogleStorage interface.
func (n NoopGoogleStorage) Delete(ctx context.Context, path string) error {
	if n.Err == nil {
		panic("must not be called")
	}
	return n.Err
}

// Publish is part of gs.GoogleStorage interface.
func (n NoopGoogleStorage) Publish(ctx context.Context, dst, src string, srcGen int64) error {
	if n.Err == nil {
		panic("must not be called")
	}
	return n.Err
}

// StartUpload is part of gs.GoogleStorage interface.
func (n NoopGoogleStorage) StartUpload(ctx context.Context, path string) (uploadURL string, err error) {
	if n.Err == nil {
		panic("must not be called")
	}
	return "", n.Err
}

// CancelUpload is part of gs.GoogleStorage interface.
func (n NoopGoogleStorage) CancelUpload(ctx context.Context, uploadURL string) error {
	if n.Err == nil {
		panic("must not be called")
	}
	return n.Err
}

// Reader is part of gs.GoogleStorage interface.
func (n NoopGoogleStorage) Reader(ctx context.Context, path string, gen, minSpeed int64) (gs.Reader, error) {
	if n.Err == nil {
		panic("must not be called")
	}
	return nil, n.Err
}

// MockGSReader implements gs.Reader on top of a regular io.ReaderAt.
type MockGSReader struct {
	io.ReaderAt

	Len int64
	Gen int64
}

// Size is part of gs.Reader interface.
func (m *MockGSReader) Size() int64 { return m.Len }

// Generation is part of gs.Reader interface.
func (m *MockGSReader) Generation() int64 { return m.Gen }

// NewMockGSReader constructs MockGSReader from a byte slice.
func NewMockGSReader(data []byte) *MockGSReader {
	return &MockGSReader{
		ReaderAt: bytes.NewReader(data),
		Len:      int64(len(data)),
		Gen:      1,
	}
}
