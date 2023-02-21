// Copyright 2023 The LUCI Authors.
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

// Package testsupport contains helper functions for testing auth service.
package testsupport

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
)

// BuildTargz builds a tar bundle.
func BuildTargz(files map[string][]byte) []byte {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, body := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0600,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil
		}
		if _, err := tw.Write(body); err != nil {
			return nil
		}
	}
	if err := tw.Flush(); err != nil {
		return nil
	}
	if err := tw.Close(); err != nil {
		return nil
	}
	if err := gzw.Flush(); err != nil {
		return nil
	}
	if err := gzw.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}
