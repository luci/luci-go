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

package policy

import (
	"bytes"
	"encoding/gob"
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestConfigBundle(t *testing.T) {
	ftt.Run("Empty map", t, func(t *ftt.Test) {
		var b ConfigBundle
		blob, err := serializeBundle(b)
		assert.Loosely(t, err, should.BeNil)

		b, unknown, err := deserializeBundle(blob)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, unknown, should.BeNil)
		assert.Loosely(t, len(b), should.BeZero)
	})

	ftt.Run("Non-empty map", t, func(t *ftt.Test) {
		// We use well-known proto types in this test to avoid depending on some
		// other random proto messages. It doesn't matter what proto messages are
		// used here.
		b1 := ConfigBundle{
			"a": &timestamppb.Timestamp{Seconds: 1},
			"b": &durationpb.Duration{Seconds: 2},
		}
		blob, err := serializeBundle(b1)
		assert.Loosely(t, err, should.BeNil)

		b2, unknown, err := deserializeBundle(blob)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, unknown, should.BeNil)
		assert.Loosely(t, b2, should.HaveLength(len(b1)))
		for k := range b2 {
			assert.Loosely(t, b2[k], should.Match(b1[k]))
		}
	})

	ftt.Run("Unknown proto", t, func(t *ftt.Test) {
		items := []blobWithType{
			{"abc", "unknown.type", []byte("zzz")},
		}
		out := bytes.Buffer{}
		assert.Loosely(t, gob.NewEncoder(&out).Encode(items), should.BeNil)

		b, unknown, err := deserializeBundle(out.Bytes())
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, unknown, should.Match(items))
		assert.Loosely(t, len(b), should.BeZero)
	})

	ftt.Run("Rejects nil", t, func(t *ftt.Test) {
		_, err := serializeBundle(ConfigBundle{"abc": nil})
		assert.Loosely(t, err, should.NotBeNil)
	})
}
