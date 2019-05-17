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

package iotools

import (
	"net/http"
)

// ResponseWriter wraps a given http.ResponseWriter, records its status code and
// response size.
//
// Assumes all writes are externally synchronized.
type ResponseWriter struct {
	rw     http.ResponseWriter
	writer CountingWriter
	status int
}

// NewResponseWriter constructs a ResponseWriter that wraps given 'rw' and
// tracks how much data was written to it and what status code was set.
func NewResponseWriter(rw http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{
		rw:     rw,
		writer: CountingWriter{Writer: rw},
		status: http.StatusOK,
	}
}

// ResponseSize is size of the response body written so far.
func (rw *ResponseWriter) ResponseSize() int64 { return rw.writer.Count }

// Status is the HTTP status code set in the response.
func (rw *ResponseWriter) Status() int { return rw.status }

// http.ResponseWriter interface.

// Header returns the header map that will be sent by WriteHeader.
func (rw *ResponseWriter) Header() http.Header { return rw.rw.Header() }

// Write writes the data to the connection as part of an HTTP reply.
func (rw *ResponseWriter) Write(buf []byte) (int, error) { return rw.writer.Write(buf) }

// WriteHeader sends an HTTP response header with the provided status code.
func (rw *ResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.rw.WriteHeader(code)
}

// http.Flusher interface.

// Flush sends any buffered data to the client.
func (rw *ResponseWriter) Flush() {
	if f, ok := rw.rw.(http.Flusher); ok {
		f.Flush()
	}
}
