// Copyright 2025 The LUCI Authors.
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

package botstate

import (
	"context"
	"encoding/json"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestDict(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		d := Dict{}

		assert.That(t, d.String(), should.Equal("<empty>"))
		assert.That(t, d.Equal(d), should.BeTrue)
		assert.NoErr(t, d.Unseal())

		runDatastoreTest(t, d, nil)
		runJSONTest(t, d)
	})

	t.Run("Non-empty", func(t *testing.T) {
		blob := []byte(`{"str": "val", "bool": true, "any": [1, 2, 3]}`)

		d := Dict{JSON: blob}

		assert.That(t, d.String(), should.Equal(string(blob)))
		assert.That(t, d.Equal(d), should.BeTrue)
		assert.NoErr(t, d.Unseal())

		runDatastoreTest(t, d, blob)
		runJSONTest(t, d)

		raw, err := d.ReadRaw("str")
		assert.NoErr(t, err)
		assert.That(t, string(raw), should.Equal(`"val"`))

		raw, err = d.ReadRaw("missing")
		assert.NoErr(t, err)
		assert.Loosely(t, raw, should.BeNil)

		str := ""
		assert.NoErr(t, d.Read("str", &str))
		assert.That(t, str, should.Equal("val"))
		assert.That(t, d.MustReadString("str"), should.Equal("val"))
		assert.That(t, d.MustReadString("missing"), should.Equal(""))
		assert.That(t, d.MustReadString("bool"), should.Equal(""))

		bool := false
		assert.NoErr(t, d.Read("bool", &bool))
		assert.That(t, bool, should.BeTrue)
		assert.That(t, d.MustReadBool("bool"), should.BeTrue)
		assert.That(t, d.MustReadBool("missing"), should.BeFalse)
		assert.That(t, d.MustReadBool("str"), should.BeFalse)

		var list []int64
		assert.NoErr(t, d.Read("any", &list))
		assert.That(t, list, should.Match([]int64{1, 2, 3}))
	})

	t.Run("Broken", func(t *testing.T) {
		blob := []byte(`not JSON`)

		d := Dict{JSON: blob}

		// Still can be round-tripped through datastore.
		runDatastoreTest(t, d, blob)

		val := ""
		assert.Loosely(t, d.Read("key", &val), should.NotBeNil)
		assert.That(t, d.MustReadString("key"), should.Equal(""))
		assert.That(t, d.MustReadBool("key"), should.BeFalse)
	})

	t.Run("Equal", func(t *testing.T) {
		eq := func(a, b string) bool {
			return (Dict{JSON: []byte(a)}).Equal(Dict{JSON: []byte(b)})
		}

		assert.That(t, eq(`not json`, `not json`), should.BeTrue)
		assert.That(t, eq(`not json`, `another not json`), should.BeFalse)
		assert.That(t, eq(``, `{}`), should.BeTrue)
		assert.That(t, eq(``, `{"a": 123}`), should.BeFalse)
		assert.That(t, eq(`{"a": 123}`, `{"a": 123}`), should.BeTrue)
		assert.That(t, eq(`{"a":      123}`, `{"a": 123}`), should.BeTrue)
		assert.That(t, eq(`{"a": 888}`, `{"a": 123}`), should.BeFalse)
	})

	t.Run("Editing noop", func(t *testing.T) {
		blob := []byte(`{"str": "val"}`)

		d := Dict{JSON: blob}

		out, err := Edit(d, func(d *EditableDict) error {
			var str string
			assert.NoErr(t, d.Read("str", &str))
			assert.That(t, str, should.Equal("val"))
			return nil
		})

		assert.NoErr(t, err)
		assert.That(t, out.JSON, should.Match(blob))
	})

	t.Run("Editing for real", func(t *testing.T) {
		blob := []byte(`{"str": "val"}`)

		d := Dict{JSON: blob}

		out, err := Edit(d, func(d *EditableDict) error {
			assert.NoErr(t, d.Write("str", "another val"))
			var str string
			assert.NoErr(t, d.Read("str", &str))
			assert.That(t, str, should.Equal("another val"))
			return nil
		})

		assert.NoErr(t, err)
		assert.That(t, out.JSON, should.Match([]byte(`{
  "str": "another val"
}`)))

		assert.That(t, out.MustReadString("str"), should.Equal("another val"))
	})
}

func runDatastoreTest(t *testing.T, d Dict, asBlob []byte) {
	ctx := memory.Use(context.Background())

	store := testEntityDict{ID: 1, State: d}
	assert.NoErr(t, datastore.Put(ctx, &store))

	// Can be loaded back.
	load := testEntityDict{ID: 1}
	assert.NoErr(t, datastore.Get(ctx, &load))
	assert.That(t, store, should.Match(load))

	// Serialized as PTBytes property.
	loadBytes := testEntityBlob{ID: 1}
	assert.NoErr(t, datastore.Get(ctx, &loadBytes))
	assert.That(t, loadBytes.State, should.Match(asBlob))
}

func runJSONTest(t *testing.T, d Dict) {
	store := testJSON{State: d}

	blob, err := json.Marshal(&store)
	assert.NoErr(t, err)

	load := testJSON{}
	assert.NoErr(t, json.Unmarshal(blob, &load))
	assert.That(t, store, should.Match(load))
}

type testEntityDict struct {
	Kind  string `gae:"$kind,testEntity"`
	ID    int64  `gae:"$id"`
	State Dict   `gae:"state"`
}

type testEntityBlob struct {
	Kind  string `gae:"$kind,testEntity"`
	ID    int64  `gae:"$id"`
	State []byte `gae:"state"`
}

type testJSON struct {
	State Dict
}
