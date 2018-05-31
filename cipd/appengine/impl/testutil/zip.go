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

package testutil

import (
	"archive/zip"
	"bytes"
	"sort"
)

// MakeZip produces a zip archive with given files.
//
// Can be used to test reading of CIPD packages.
func MakeZip(files map[string]string) []byte {
	buf := bytes.NewBuffer(nil)
	w := zip.NewWriter(buf)

	names := make([]string, 0, len(files))
	for k := range files {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, name := range names {
		fw, err := w.Create(name)
		if err != nil {
			panic(err)
		}
		_, err = fw.Write([]byte(files[name]))
		if err != nil {
			panic(err)
		}
	}

	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
