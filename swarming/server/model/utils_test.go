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

	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestToJSONProperty(t *testing.T) {
	t.Parallel()

	Convey("With a value", t, func() {
		p, err := ToJSONProperty(map[string]string{"a": "b"})
		So(err, ShouldBeNil)
		So(p.Type(), ShouldEqual, datastore.PTString)
		So(p.Value().(string), ShouldEqual, `{"a":"b"}`)
	})

	Convey("With empty map", t, func() {
		p, err := ToJSONProperty(map[string]string{})
		So(err, ShouldBeNil)
		So(p.Type(), ShouldEqual, datastore.PTNull)
	})

	Convey("With empty list", t, func() {
		p, err := ToJSONProperty([]string{})
		So(err, ShouldBeNil)
		So(p.Type(), ShouldEqual, datastore.PTNull)
	})

	Convey("With nil", t, func() {
		p, err := ToJSONProperty(nil)
		So(err, ShouldBeNil)
		So(p.Type(), ShouldEqual, datastore.PTNull)
	})

	Convey("With typed nil", t, func() {
		var m map[string]string
		p, err := ToJSONProperty(m)
		So(err, ShouldBeNil)
		So(p.Type(), ShouldEqual, datastore.PTNull)
	})
}

func TestFromJSONProperty(t *testing.T) {
	t.Parallel()

	Convey("Null", t, func() {
		var v map[string]string
		So(FromJSONProperty(datastore.MkProperty(nil), &v), ShouldBeNil)
		So(v, ShouldResemble, map[string]string(nil))
	})

	Convey("Empty", t, func() {
		var v map[string]string
		So(FromJSONProperty(datastore.MkProperty(""), &v), ShouldBeNil)
		So(v, ShouldResemble, map[string]string(nil))
	})

	Convey("Null", t, func() {
		var v map[string]string
		So(FromJSONProperty(datastore.MkProperty("null"), &v), ShouldBeNil)
		So(v, ShouldResemble, map[string]string(nil))
	})

	Convey("Bytes", t, func() {
		var v map[string]string
		So(FromJSONProperty(datastore.MkProperty([]byte(`{"a":"b"}`)), &v), ShouldBeNil)
		So(v, ShouldResemble, map[string]string{"a": "b"})
	})

	Convey("String", t, func() {
		var v map[string]string
		So(FromJSONProperty(datastore.MkProperty(`{"a":"b"}`), &v), ShouldBeNil)
		So(v, ShouldResemble, map[string]string{"a": "b"})
	})

	Convey("Compressed", t, func() {
		var v map[string]string
		So(FromJSONProperty(datastore.MkProperty(deflate([]byte(`{"a":"b"}`))), &v), ShouldBeNil)
		So(v, ShouldResemble, map[string]string{"a": "b"})
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

	Convey("Works", t, func() {
		for _, cs := range cases {
			So(dimensionsFlatToPb(cs.flat), ShouldResembleProto, cs.list)
		}
	})
}

func TestMapToStringListPair(t *testing.T) {
	t.Parallel()

	Convey("ok", t, func() {
		m := map[string][]string{
			"key2": []string{"val3", "val4"},
			"key1": []string{"val1", "val2"},
			"key3": []string{"val3", "val2"},
		}
		// Since iteration over a map randomizes the keys in Go, we need to
		// assert that all the items are in the []*apipb.StringListPair and that
		// the lengths match. If we compared tp an apipb.StringListPair type directly,
		// the test would be flakey.
		Convey("unsorted", func() {
			spl := MapToStringListPair(m, false)
			So(len(spl), ShouldEqual, len(m))
			So(&apipb.StringListPair{
				Key:   "key2",
				Value: []string{"val3", "val4"},
			}, ShouldBeIn, spl)
			So(&apipb.StringListPair{
				Key:   "key1",
				Value: []string{"val1", "val2"},
			}, ShouldBeIn, spl)
			So(&apipb.StringListPair{
				Key:   "key3",
				Value: []string{"val3", "val2"},
			}, ShouldBeIn, spl)
		})

		Convey("sorted", func() {
			So(MapToStringListPair(m, true), ShouldEqual, []*apipb.StringListPair{
				{Key: "key1", Value: []string{"val1", "val2"}},
				{Key: "key2", Value: []string{"val3", "val4"}},
				{Key: "key3", Value: []string{"val3", "val2"}},
			})
		})
	})

	Convey("empty", t, func() {
		m := map[string][]string{}
		So(MapToStringListPair(m, false), ShouldBeNil)
	})
	Convey("nil", t, func() {
		So(MapToStringListPair(nil, false), ShouldBeNil)
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
