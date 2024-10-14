// Copyright 2016 The LUCI Authors.
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
	"encoding/base64"
	"net/http"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"google.golang.org/grpc/metadata"
)

func TestBinaryHeader(t *testing.T) {
	t.Parallel()

	ftt.Run("from headers", t, func(t *ftt.Test) {
		t.Run("regular", func(t *ftt.Test) {
			md, err := headersIntoMetadata(http.Header{
				"Key":         {"v1", "v2"},
				"Another-Key": {"v3"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, md, should.Resemble(metadata.MD{
				"key":         {"v1", "v2"},
				"another-key": {"v3"},
			}))
		})

		t.Run("binary", func(t *ftt.Test) {
			data := []byte{10}
			md, err := headersIntoMetadata(http.Header{
				"Key-Bin": {base64.StdEncoding.EncodeToString(data)},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, md, should.Resemble(metadata.MD{
				"key-bin": {string(data)},
			}))
		})

		t.Run("binary invalid", func(t *ftt.Test) {
			_, err := headersIntoMetadata(http.Header{
				"Key-Bin": {"Z"},
			})
			assert.Loosely(t, err, should.ErrLike("illegal base64 data at input byte 0"))
		})

		t.Run("reserved", func(t *ftt.Test) {
			md, err := headersIntoMetadata(http.Header{
				"Content-Type": {"zzz"},
				"X-Prpc-Zzz":   {"zzz"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, md, should.Resemble(metadata.MD(nil)))
		})
	})

	ftt.Run("to headers", t, func(t *ftt.Test) {
		t.Run("regular", func(t *ftt.Test) {
			h := http.Header{}
			err := metaIntoHeaders(metadata.MD{
				"key":         {"v1", "v2"},
				"another-key": {"v3"},
			}, h)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, h, should.Resemble(http.Header{
				"Key":         {"v1", "v2"},
				"Another-Key": {"v3"},
			}))
		})

		t.Run("binary", func(t *ftt.Test) {
			data := []byte{10}
			h := http.Header{}
			err := metaIntoHeaders(metadata.MD{
				"key-bin": {string(data)},
			}, h)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, h, should.Resemble(http.Header{
				"Key-Bin": {base64.StdEncoding.EncodeToString(data)},
			}))
		})

		t.Run("reserved", func(t *ftt.Test) {
			h := http.Header{}
			err := metaIntoHeaders(metadata.MD{
				"content-type": {"zzz"},
			}, h)
			assert.Loosely(t, err, should.ErrLike("using reserved metadata key"))
		})
	})
}
