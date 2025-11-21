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
	"slices"
	"testing"

	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
			assert.Loosely(t, md, should.Match(metadata.MD{
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
			assert.Loosely(t, md, should.Match(metadata.MD{
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
			assert.Loosely(t, md, should.Match(metadata.MD(nil)))
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
			assert.Loosely(t, h, should.Match(http.Header{
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
			assert.Loosely(t, h, should.Match(http.Header{
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

func TestIterHeaderValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name: "empty 1",
		},
		{
			name:  "empty 2",
			input: []string{""},
			want:  []string{""},
		},
		{
			name:  "simple",
			input: []string{"abc", "def"},
			want:  []string{"abc", "def"},
		},
		{
			name:  "split",
			input: []string{"123,456,789", "abc"},
			want:  []string{"123", "456", "789", "abc"},
		},
		{
			name:  "empty terms 1",
			input: []string{"123,,"},
			want:  []string{"123", "", ""},
		},
		{
			name:  "empty terms 2",
			input: []string{"123,,456"},
			want:  []string{"123", "", "456"},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			assert.That(t, slices.Collect(iterHeaderValues(cs.input)), should.Match(cs.want))
		})
	}
}
