// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pubsub

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResource(t *testing.T) {
	for _, r := range []string{
		"googResource",
		"yo",
		strings.Repeat("a", 256),
		"1ring",
		"allAboutThe$",
		"アップ",
	} {
		Convey(fmt.Sprintf(`An invalid resource [%s] will not validate.`, r), t, func() {
			So(validateResource(r), ShouldNotBeNil)
		})
	}

	for _, r := range []string{
		"AAA",
		"testResource",
		strings.Repeat("a", 255),
		"myResource-a_b.c~d+e%%f",
	} {
		Convey(fmt.Sprintf(`A valid resource [%s] will validate.`, r), t, func() {
			So(validateResource(r), ShouldBeNil)
		})
	}

	Convey(`Can calculate a resource path.`, t, func() {
		So(resourcePath("proj", "subscriptions", "mySub"), ShouldEqual, "projects/proj/subscriptions/mySub")
	})
}
