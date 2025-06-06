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

package cipd

import (
	"context"
	"hash"
	"io"
	"sync"
	"sync/atomic"

	"go.chromium.org/luci/common/errors"
)

// This file has no tests, but contains definition of a mock for 'storage'
// interface used to upload and download files from Google Storage.

type mockedStorage struct {
	l             sync.RWMutex
	store         map[string]string // URL -> data
	err           error
	downloadCount int64
}

func (s *mockedStorage) getStored(url string) string {
	s.l.RLock()
	defer s.l.RUnlock()
	return s.store[url]
}

func (s *mockedStorage) putStored(url, data string) {
	s.l.Lock()
	defer s.l.Unlock()
	if s.store == nil {
		s.store = make(map[string]string, 1)
	}
	s.store[url] = data
}

func (s *mockedStorage) downloads() int {
	return int(atomic.LoadInt64(&s.downloadCount))
}

func (s *mockedStorage) returnErr(err error) {
	s.err = err
}

func (s *mockedStorage) upload(ctx context.Context, url string, data io.ReadSeeker) error {
	// Mimic the real storage implementation by seeking in same patter as it does.
	if _, err := data.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if _, err := data.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if s.err != nil {
		return s.err
	}
	blob, err := io.ReadAll(data)
	if err == nil {
		s.putStored(url, string(blob))
	}
	return err
}

func (s *mockedStorage) download(ctx context.Context, url string, output io.WriteSeeker, h hash.Hash) error {
	atomic.AddInt64(&s.downloadCount, 1)

	// Mimic the real storage behavioral patterns.
	h.Reset()
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if s.err != nil {
		return s.err
	}

	body := s.getStored(url)
	if body == "" {
		return errors.New("mocked downloaded error")
	}

	_, err := io.MultiWriter(output, h).Write([]byte(body))
	return err
}
