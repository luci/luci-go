// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"fmt"
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestProjectName(t *testing.T) {
	t.Parallel()

	Convey(`Testing valid project names`, t, func() {
		for _, testCase := range []ProjectName{
			"a",
			"foo_bar-baz-059",
		} {
			Convey(fmt.Sprintf(`Project name %q is valid`, testCase), func() {
				So(testCase.Validate(), ShouldBeNil)
			})
		}
	})

	Convey(`Testing invalid project names`, t, func() {
		for _, testCase := range []struct {
			v          ProjectName
			errorsLike string
		}{
			{"", "cannot have empty name"},
			{"foo/bar", "invalid character"},
			{"_name", "must begin with a letter"},
			{"1eet", "must begin with a letter"},
		} {
			Convey(fmt.Sprintf(`Project name %q fails with error %q`, testCase.v, testCase.errorsLike), func() {
				So(testCase.v.Validate(), ShouldErrLike, testCase.errorsLike)
			})
		}
	})
}
