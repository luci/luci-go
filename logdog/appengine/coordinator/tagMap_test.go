// Copyright 2015 The LUCI Authors.
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

package coordinator

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	ds "go.chromium.org/gae/service/datastore"
)

func TestTagMap(t *testing.T) {
	t.Parallel()

	Convey(`Will return nil when encoding an empty TagMap.`, t, func() {
		tm := TagMap(nil)
		prop, err := tm.ToProperty()
		So(err, ShouldBeNil)
		So(prop, ShouldResemble, ds.MkPropertyNI(nil))
	})

	Convey(`Will return nil when decoding an empty property set.`, t, func() {
		tm, err := tagMapFromProperties([]ds.Property(nil))
		So(err, ShouldBeNil)
		So(tm, ShouldBeNil)
	})

	Convey(`Will encode tags as both value and presence tags.`, t, func() {
		tm := TagMap{
			"foo":  "bar",
			"baz":  "qux",
			"quux": "",
		}

		prop, err := tm.ToProperty()
		So(err, ShouldBeNil)
		So(prop, ShouldResemble, ds.MkPropertyNI(
			[]byte(`{"baz":"qux","foo":"bar","quux":""}`),
		))

		Convey(`Can decode tags.`, func() {
			dtm := TagMap{}
			So(dtm.FromProperty(prop), ShouldBeNil)
			So(dtm, ShouldResemble, tm)
		})
	})

	Convey(`Will decode old properties into new map.`, t, func() {
		ls := &LogStream{noDSValidate: true}
		err := ls.Load(ds.PropertyMap{
			"_Tags": sps(
				encodeKey("foo"),
				encodeKey("foo=bar"),
				encodeKey("quux"),
				encodeKey("quux="),
			),
		})
		So(err, ShouldBeNil)
		So(ls.Tags, ShouldResemble, TagMap{
			"foo":  "bar",
			"quux": "",
		})

		out, err := ls.Save(false)
		So(err, ShouldBeNil)
		So(out["Tags"], ShouldResemble, ds.MkPropertyNI(
			[]byte(`{"foo":"bar","quux":""}`),
		))
	})

	Convey(`Can load from datastore`, t, func() {
		ls := &LogStream{noDSValidate: true}
		err := ls.Load(ds.PropertyMap{
			"Tags": ds.PropertySlice{ds.MkPropertyNI([]byte(`{"foo":"bar"}`))},
		})
		So(err, ShouldBeNil)
		So(ls.Tags, ShouldResemble, TagMap{"foo": "bar"})
	})
}
