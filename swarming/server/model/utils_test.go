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

	. "github.com/smartystreets/goconvey/convey"
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
