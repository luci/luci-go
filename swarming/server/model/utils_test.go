// Copyright 2023 The LUCI Authors.
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

package model

import (
	"bytes"
	"compress/zlib"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestToJSONProperty(t *testing.T) {
	t.Parallel()

	ftt.Run("With a value", t, func(t *ftt.Test) {
		p, err := ToJSONProperty(map[string]string{"a": "b"})
		assert.NoErr(t, err)
		assert.Loosely(t, p.Type(), should.Equal(datastore.PTString))
		assert.Loosely(t, p.Value().(string), should.Equal(`{"a":"b"}`))
	})

	ftt.Run("With empty map", t, func(t *ftt.Test) {
		p, err := ToJSONProperty(map[string]string{})
		assert.NoErr(t, err)
		assert.Loosely(t, p.Type(), should.Equal(datastore.PTNull))
	})

	ftt.Run("With empty list", t, func(t *ftt.Test) {
		p, err := ToJSONProperty([]string{})
		assert.NoErr(t, err)
		assert.Loosely(t, p.Type(), should.Equal(datastore.PTNull))
	})

	ftt.Run("With nil", t, func(t *ftt.Test) {
		p, err := ToJSONProperty(nil)
		assert.NoErr(t, err)
		assert.Loosely(t, p.Type(), should.Equal(datastore.PTNull))
	})

	ftt.Run("With typed nil", t, func(t *ftt.Test) {
		var m map[string]string
		p, err := ToJSONProperty(m)
		assert.NoErr(t, err)
		assert.Loosely(t, p.Type(), should.Equal(datastore.PTNull))
	})
}

func TestFromJSONProperty(t *testing.T) {
	t.Parallel()

	ftt.Run("Null", t, func(t *ftt.Test) {
		var v map[string]string
		assert.NoErr(t, FromJSONProperty(datastore.MkProperty(nil), &v))
		assert.Loosely(t, v, should.Match(map[string]string(nil)))
	})

	ftt.Run("Empty", t, func(t *ftt.Test) {
		var v map[string]string
		assert.NoErr(t, FromJSONProperty(datastore.MkProperty(""), &v))
		assert.Loosely(t, v, should.Match(map[string]string(nil)))
	})

	ftt.Run("Null", t, func(t *ftt.Test) {
		var v map[string]string
		assert.NoErr(t, FromJSONProperty(datastore.MkProperty("null"), &v))
		assert.Loosely(t, v, should.Match(map[string]string(nil)))
	})

	ftt.Run("Bytes", t, func(t *ftt.Test) {
		var v map[string]string
		assert.NoErr(t, FromJSONProperty(datastore.MkProperty([]byte(`{"a":"b"}`)), &v))
		assert.Loosely(t, v, should.Match(map[string]string{"a": "b"}))
	})

	ftt.Run("String", t, func(t *ftt.Test) {
		var v map[string]string
		assert.NoErr(t, FromJSONProperty(datastore.MkProperty(`{"a":"b"}`), &v))
		assert.Loosely(t, v, should.Match(map[string]string{"a": "b"}))
	})

	ftt.Run("Compressed", t, func(t *ftt.Test) {
		var v map[string]string
		assert.NoErr(t, FromJSONProperty(datastore.MkProperty(deflate([]byte(`{"a":"b"}`))), &v))
		assert.Loosely(t, v, should.Match(map[string]string{"a": "b"}))
	})
}

func TestDimensionsFlatToPb(t *testing.T) {
	t.Parallel()

	type testCase struct {
		flat []string
		list []*apipb.StringListPair
	}
	cases := []testCase{
		{
			flat: nil,
			list: nil,
		},
		{
			flat: []string{"a:1"},
			list: []*apipb.StringListPair{
				{Key: "a", Value: []string{"1"}},
			},
		},
		{
			flat: []string{"a:1", "a:1"},
			list: []*apipb.StringListPair{
				{Key: "a", Value: []string{"1"}},
			},
		},
		{
			flat: []string{"a:1", "a:2"},
			list: []*apipb.StringListPair{
				{Key: "a", Value: []string{"1", "2"}},
			},
		},
		{
			flat: []string{"a:2", "a:1"},
			list: []*apipb.StringListPair{
				{Key: "a", Value: []string{"1", "2"}},
			},
		},
		{
			flat: []string{"a:1", "b:2"},
			list: []*apipb.StringListPair{
				{Key: "a", Value: []string{"1"}},
				{Key: "b", Value: []string{"2"}},
			},
		},
		{
			flat: []string{"a:1", "a:2", "b:1", "b:2"},
			list: []*apipb.StringListPair{
				{Key: "a", Value: []string{"1", "2"}},
				{Key: "b", Value: []string{"1", "2"}},
			},
		},
		{
			flat: []string{"b:1", "b:2", "a:1", "a:2"},
			list: []*apipb.StringListPair{
				{Key: "a", Value: []string{"1", "2"}},
				{Key: "b", Value: []string{"1", "2"}},
			},
		},
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		for _, cs := range cases {
			assert.Loosely(t, DimensionsFlatToPb(cs.flat), should.Match(cs.list))
		}
	})
}

func TestMapToStringListPair(t *testing.T) {
	t.Parallel()

	ftt.Run("ok", t, func(t *ftt.Test) {
		m := map[string][]string{
			"key2": {"val3", "val4"},
			"key1": {"val1", "val2"},
			"key3": {"val3", "val2"},
		}
		// Since iteration over a map randomizes the keys in Go, we need to
		// assert that all the items are in the []*apipb.StringListPair and that
		// the lengths match. If we compared tp an apipb.StringListPair type directly,
		// the test would be flakey.
		t.Run("unsorted", func(t *ftt.Test) {
			spl := MapToStringListPair(m, false)
			assert.Loosely(t, len(spl), should.Equal(len(m)))
			assert.Loosely(t, &apipb.StringListPair{
				Key:   "key2",
				Value: []string{"val3", "val4"},
			}, should.MatchIn(spl))
			assert.Loosely(t, &apipb.StringListPair{
				Key:   "key1",
				Value: []string{"val1", "val2"},
			}, should.MatchIn(spl))
			assert.Loosely(t, &apipb.StringListPair{
				Key:   "key3",
				Value: []string{"val3", "val2"},
			}, should.MatchIn(spl))
		})

		t.Run("sorted", func(t *ftt.Test) {
			assert.Loosely(t, MapToStringListPair(m, true), should.Match([]*apipb.StringListPair{
				{Key: "key1", Value: []string{"val1", "val2"}},
				{Key: "key2", Value: []string{"val3", "val4"}},
				{Key: "key3", Value: []string{"val3", "val2"}},
			}))
		})
	})

	ftt.Run("empty", t, func(t *ftt.Test) {
		m := map[string][]string{}
		assert.Loosely(t, MapToStringListPair(m, false), should.BeNil)
	})
	ftt.Run("nil", t, func(t *ftt.Test) {
		assert.Loosely(t, MapToStringListPair(nil, false), should.BeNil)
	})
}

func deflate(blob []byte) []byte {
	out := bytes.NewBuffer(nil)
	w := zlib.NewWriter(out)
	if _, err := w.Write(blob); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return out.Bytes()
}
