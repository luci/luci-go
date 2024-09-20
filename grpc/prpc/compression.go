// Copyright 2021 The LUCI Authors.
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

package prpc

import (
	"bytes"
	"io"
	"sync"

	"github.com/klauspost/compress/gzip"
)

// gzipThreshold is the threshold to compress a blob.
// If the blob is larger than this, then compress it.
// The value is derived from
// https://webmasters.stackexchange.com/questions/31750/what-is-recommended-minimum-object-size-for-gzip-performance-benefits
const gzipThreshold = 1024

var (
	gzipWriters sync.Pool
	gzipReaders sync.Pool
)

func getGZipWriter(w io.Writer) *gzip.Writer {
	if gw, _ := gzipWriters.Get().(*gzip.Writer); gw != nil {
		gw.Reset(w)
		return gw
	}
	return gzip.NewWriter(w)
}

func returnGZipWriter(gw *gzip.Writer) {
	gzipWriters.Put(gw)
}

func getGZipReader(r io.Reader) (*gzip.Reader, error) {
	if gr, _ := gzipReaders.Get().(*gzip.Reader); gr != nil {
		if err := gr.Reset(r); err != nil {
			gzipReaders.Put(gr) // it is still good for reuse, even on errors
			return nil, err
		}
		return gr, nil
	}
	return gzip.NewReader(r)
}

func returnGZipReader(gr *gzip.Reader) {
	gzipReaders.Put(gr)
}

// compressBlob compresses data using gzip.
func compressBlob(data []byte) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	gz := getGZipWriter(buf)
	defer returnGZipWriter(gz)
	if _, err := gz.Write(data); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
