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

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfigBundle(t *testing.T) {
	Convey("Empty map", t, func() {
		var b ConfigBundle
		blob, err := serializeBundle(b)
		So(err, ShouldBeNil)

		b, unknown, err := deserializeBundle(blob)
		So(err, ShouldBeNil)
		So(unknown, ShouldEqual, nil)
		So(len(b), ShouldEqual, 0)
	})

	Convey("Non-empty map", t, func() {
		// We use well-known proto types in this test to avoid depending on some
		// other random proto messages. It doesn't matter what proto messages are
		// used here.
		b1 := ConfigBundle{
			"a": &timestamp.Timestamp{Seconds: 1},
			"b": &duration.Duration{Seconds: 2},
		}
		blob, err := serializeBundle(b1)
		So(err, ShouldBeNil)

		b2, unknown, err := deserializeBundle(blob)
		So(err, ShouldBeNil)
		So(unknown, ShouldEqual, nil)
		So(b2, ShouldHaveLength, len(b1))
		for k := range b2 {
			So(b2[k], ShouldResembleProto, b1[k])
		}
	})

	Convey("Unknown proto", t, func() {
		items := []blobWithType{
			{"abc", "unknown.type", []byte("zzz")},
		}
		out := bytes.Buffer{}
		So(gob.NewEncoder(&out).Encode(items), ShouldBeNil)

		b, unknown, err := deserializeBundle(out.Bytes())
		So(err, ShouldBeNil)
		So(unknown, ShouldResemble, items)
		So(len(b), ShouldEqual, 0)
	})

	Convey("Rejects nil", t, func() {
		_, err := serializeBundle(ConfigBundle{"abc": nil})
		So(err, ShouldNotBeNil)
	})
}
