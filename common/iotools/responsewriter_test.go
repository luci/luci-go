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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestResponseWriter(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		rec := httptest.NewRecorder()
		rw := NewResponseWriter(rec)

		assert.Loosely(t, rw.Status(), should.Equal(http.StatusOK))

		rw.Header().Set("Foo", "bar")
		rw.WriteHeader(http.StatusNotFound)
		rw.Write([]byte("1234"))
		rw.Write([]byte("5678"))
		rw.Flush()

		assert.Loosely(t, rw.ResponseSize(), should.Equal(8))
		assert.Loosely(t, rw.Status(), should.Equal(http.StatusNotFound))

		assert.Loosely(t, rec.Body.Len(), should.Equal(8))
		assert.Loosely(t, rec.Code, should.Equal(http.StatusNotFound))
		assert.Loosely(t, rec.Flushed, should.BeTrue)
	})
}
