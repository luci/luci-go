// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package policy

import (
	"bytes"
	"encoding/gob"
	"testing"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"

	. "github.com/smartystreets/goconvey/convey"
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
		So(b2, ShouldResemble, b1)
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
