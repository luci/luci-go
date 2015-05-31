// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"
	"infra/gae/libs/wrapper"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"

	"appengine/datastore"
)

func shouldEqualKey(ai interface{}, bis ...interface{}) string {
	if len(bis) != 1 {
		return "too many bees!"
	}
	a, b := ai.(*datastore.Key), bis[0].(*datastore.Key)
	if !a.Equal(b) {
		return fmt.Sprintf("Expected: %s\nActual:   %s", b.String(), a.String())
	}
	return ""
}

func TestKeyBinaryStuff(t *testing.T) {
	t.Parallel()

	Convey("Key binary encoding", t, func() {
		c := Use(context.Background())
		c, err := wrapper.GetGI(c).Namespace("bobspace")
		So(err, ShouldBeNil)
		ds := wrapper.GetDS(c)

		k := ds.NewKey("Bunny", "", 10, ds.NewKey("Parent", "Cat", 0, nil))
		So(k.Namespace(), ShouldEqual, "bobspace")

		b := &bytes.Buffer{}
		writeKey(b, withNS, k)

		newk, err := readKey(b, withNS, "")
		So(err, ShouldBeNil)
		So(k.Namespace(), ShouldEqual, "bobspace")
		So(newk, shouldEqualKey, k)
		So(b.Bytes(), ShouldBeEmpty)
	})
}
