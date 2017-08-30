// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package access

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	Convey("Description", t, func() {
		validDesc := DescriptionResponse_ResourceDescription{
			Pattern: "a/{a}",
			PatternParameters: map[string]string{
				"a": "a doc",
			},
			Actions: []*DescriptionResponse_ResourceDescription_Action{
				{"VIEW_BUILD", "view build"},
				{"ADD_BUILD", "add build"},
			},
			Roles: map[string]*DescriptionResponse_ResourceDescription_Role{
				"READER": {
					AllowedActions: []string{"VIEW_BUILD"},
				},
				"SCHEDULER": {
					AllowedActions: []string{"VIEW_BUILD", "ADD_BUILD"},
				},
			},
		}
		r := Resource{Description: validDesc}

		Convey("valid", func() {
			So(r.validate(), ShouldBeNil)
		})

		Convey("invalid action id", func() {
			r.Description.Actions[0].ActionId = ""
			So(r.validate(), ShouldErrLike, `action ID "" does not match regexp`)
		})

		Convey("undefined action", func() {
			r.Description.Roles["READER"].AllowedActions[0] = "bla"
			So(r.validate(), ShouldErrLike, `undefined action "bla"`)
		})

		Convey("duplicate action", func() {
			r.Description.Roles["SCHEDULER"].AllowedActions[1] = "VIEW_BUILD"
			So(r.validate(), ShouldErrLike, `duplicate action "VIEW_BUILD`)
		})

		Convey("param not in pattern_params", func() {
			delete(r.Description.PatternParameters, "a")
			So(r.validate(), ShouldErrLike, `parameter "a" is defined in pattern, but not in pattern_params`)
		})

		Convey("param not in pattern", func() {
			r.Description.PatternParameters["b"] = "b"
			So(r.validate(), ShouldErrLike, `parameter "b" is defined in pattern_params, but not in pattern`)
		})

		Convey("no roles", func() {
			r.Description.Roles = nil
			So(r.validate(), ShouldErrLike, `roles are not defined`)
		})

		Convey("duplicate action def", func() {
			r.Description.Actions[1].ActionId = "VIEW_BUILD"
			So(r.validate(), ShouldErrLike, `duplicate action "VIEW_BUILD"`)
		})

		Convey("invalid role id", func() {
			r.Description.Roles["x"] = &DescriptionResponse_ResourceDescription_Role{}
			So(r.validate(), ShouldErrLike, `role "x" does not match regexp`)
		})
	})
}
