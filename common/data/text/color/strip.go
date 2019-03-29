// Copyright 2019 The LUCI Authors.
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

package color

import (
	"bytes"
	"io"
)

// StripWriter strips all color codes.
type StripWriter struct {
	io.Writer
}

var colorStart = []byte("\033[")

func (w *StripWriter) Write(p []byte) (n int, err error) {
	for len(p) > 0 {
		var toWrite []byte
		start := bytes.Index(p, colorStart)
		if start != -1 {
			relEnd := bytes.IndexRune(p[start:], 'm')
			if relEnd != -1 {
				toWrite = p[:start]
				p = p[start+relEnd+1:]
			}
		}
		if toWrite == nil {
			toWrite = p
			p = nil
		}

		var wrote int
		wrote, err = w.Writer.Write(toWrite)
		n += wrote
		if err != nil {
			break
		}
	}
	return
}
