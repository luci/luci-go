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
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResponseWriter(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		rec := httptest.NewRecorder()
		rw := NewResponseWriter(rec)

		So(rw.Status(), ShouldEqual, http.StatusOK)

		rw.Header().Set("Foo", "bar")
		rw.WriteHeader(http.StatusNotFound)
		rw.Write([]byte("1234"))
		rw.Write([]byte("5678"))
		rw.Flush()

		So(rw.ResponseSize(), ShouldEqual, 8)
		So(rw.Status(), ShouldEqual, http.StatusNotFound)

		So(rec.Body.Len(), ShouldEqual, 8)
		So(rec.Code, ShouldEqual, http.StatusNotFound)
		So(rec.Flushed, ShouldBeTrue)
	})
}
