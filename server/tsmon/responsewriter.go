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

package tsmon

import (
	"net/http"

	"go.chromium.org/luci/common/iotools"
)

// responseWriter wraps a given http.ResponseWriter, records its
// status code and response size.
type responseWriter struct {
	http.ResponseWriter
	writer iotools.CountingWriter
	status int
}

func (rw *responseWriter) Write(buf []byte) (int, error) { return rw.writer.Write(buf) }
func (rw *responseWriter) Size() int64                   { return rw.writer.Count }
func (rw *responseWriter) Status() int                   { return rw.status }
func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher, and just passes through the Flush() call to
// the underlying http.ResponseWriter.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func newResponseWriter(rw http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: rw,
		writer:         iotools.CountingWriter{Writer: rw},
		status:         http.StatusOK,
	}
}
