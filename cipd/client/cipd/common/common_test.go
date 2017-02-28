// Copyright 2014 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package common

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValidatePackageName(t *testing.T) {
	t.Parallel()

	Convey("ValidatePackageName works", t, func() {
		So(ValidatePackageName("good/name"), ShouldBeNil)
		So(ValidatePackageName("good_name"), ShouldBeNil)
		So(ValidatePackageName("123-_/also/good/name"), ShouldBeNil)
		So(ValidatePackageName(""), ShouldNotBeNil)
		So(ValidatePackageName("BAD/name"), ShouldNotBeNil)
		So(ValidatePackageName("bad//name"), ShouldNotBeNil)
		So(ValidatePackageName("bad/name/"), ShouldNotBeNil)
		So(ValidatePackageName("bad.name"), ShouldNotBeNil)
		So(ValidatePackageName("/bad/name"), ShouldNotBeNil)
		So(ValidatePackageName("bad/name\nyeah"), ShouldNotBeNil)
		So(ValidatePackageName("../../yeah"), ShouldNotBeNil)
	})
}

func TestValidateInstanceID(t *testing.T) {
	t.Parallel()

	Convey("ValidateInstanceID works", t, func() {
		So(ValidateInstanceID(""), ShouldNotBeNil)
		So(ValidateInstanceID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ShouldBeNil)
		So(ValidateInstanceID("0123456789abcdefaaaaaaaaaaaaaaaaaaaaaaaa"), ShouldBeNil)
		So(ValidateInstanceID("€aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ShouldNotBeNil)
		So(ValidateInstanceID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ShouldNotBeNil)
		So(ValidateInstanceID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ShouldNotBeNil)
		So(ValidateInstanceID("gaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ShouldNotBeNil)
		So(ValidateInstanceID("AAAaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ShouldNotBeNil)
	})
}

func TestValidateInstanceTag(t *testing.T) {
	t.Parallel()

	Convey("ValidateInstanceTag works", t, func() {
		So(ValidateInstanceTag(""), ShouldNotBeNil)
		So(ValidateInstanceTag("notapair"), ShouldNotBeNil)
		So(ValidateInstanceTag(strings.Repeat("long", 200)+":abc"), ShouldNotBeNil)
		So(ValidateInstanceTag("BADKEY:value"), ShouldNotBeNil)
		So(ValidateInstanceTag("good:tag"), ShouldBeNil)
		So(ValidateInstanceTag("good_tag:"), ShouldBeNil)
		So(ValidateInstanceTag("good:tag:blah"), ShouldBeNil)
		So(ValidateInstanceTag("good_tag:asdad/asdad/adad/a\\asdasdad"), ShouldBeNil)
	})
}

func TestValidatePin(t *testing.T) {
	t.Parallel()

	Convey("ValidatePin works", t, func() {
		So(ValidatePin(Pin{"good/name", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}), ShouldBeNil)
		So(ValidatePin(Pin{"BAD/name", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}), ShouldNotBeNil)
		So(ValidatePin(Pin{"good/name", "aaaaaaaaaaa"}), ShouldNotBeNil)
	})
}

func TestValidatePackageRef(t *testing.T) {
	t.Parallel()

	Convey("ValidatePackageRef works", t, func() {
		So(ValidatePackageRef("some-ref"), ShouldBeNil)
		So(ValidatePackageRef(""), ShouldNotBeNil)
		So(ValidatePackageRef("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ShouldNotBeNil)
		So(ValidatePackageRef("good:tag"), ShouldNotBeNil)
	})
}

func TestValidateInstanceVersion(t *testing.T) {
	t.Parallel()

	Convey("ValidateInstanceVersion works", t, func() {
		So(ValidateInstanceVersion("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ShouldBeNil)
		So(ValidateInstanceVersion("good:tag"), ShouldBeNil)
		So(ValidatePackageRef("some-read"), ShouldBeNil)
		So(ValidateInstanceVersion("BADTAG:"), ShouldNotBeNil)
	})
}

func TestValidateSubdir(t *testing.T) {
	badSubdirs := []struct {
		name   string
		subdir string
		err    string
	}{
		{"windows", "folder\\thing", "backslashes not allowed"},
		{"windows drive", "c:/foo/bar", `colons are not allowed`},
		{"messy", "some/../thing", `"some/../thing" (should be "thing")`},
		{"relative", "../something", `invalid "."`},
		{"single relative", "./something", `"./something" (should be "something")`},
		{"absolute", "/etc", `absolute paths not allowed`},
		{"extra slashes", "//foo/bar", `bad subdir`},
	}

	goodSubdirs := []struct {
		name   string
		subdir string
	}{
		{"empty", ""},
		{"simple path", "some/path"},
		{"single path", "something"},
		{"spaces", "some path/with/ spaces"},
	}

	Convey("ValidtateSubdir", t, func() {
		Convey("rejects bad subdirs", func() {
			for _, tc := range badSubdirs {
				Convey(tc.name, func() {
					So(ValidateSubdir(tc.subdir), ShouldErrLike, tc.err)
				})
			}
		})

		Convey("accepts good subdirs", func() {
			for _, tc := range goodSubdirs {
				Convey(tc.name, func() {
					So(ValidateSubdir(tc.subdir), ShouldErrLike, nil)
				})
			}
		})
	})
}

func TestPinToString(t *testing.T) {
	t.Parallel()

	Convey("Pin.String works", t, func() {
		So(
			fmt.Sprintf("%s", Pin{"good/name", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}),
			ShouldEqual,
			"good/name:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	})
}

func TestGetInstanceTagKey(t *testing.T) {
	t.Parallel()

	Convey("GetInstanceTagKey works", t, func() {
		So(GetInstanceTagKey("a:b"), ShouldEqual, "a")
		So(GetInstanceTagKey("a:b:c"), ShouldEqual, "a")
		So(GetInstanceTagKey(":b"), ShouldEqual, "")
		So(GetInstanceTagKey(""), ShouldEqual, "")
	})
}

func TestPinSliceAndMap(t *testing.T) {
	t.Parallel()

	Convey("PinSlice", t, func() {
		ps := PinSlice{{"pkg2", "vers"}, {"pkg", "vers"}}

		Convey("can convert to a map", func() {
			pm := ps.ToMap()
			So(pm, ShouldResemble, PinMap{
				"pkg":  "vers",
				"pkg2": "vers",
			})

			pm["new/pkg"] = "some:tag"

			Convey("and back to a slice", func() {
				So(pm.ToSlice(), ShouldResemble, PinSlice{
					{"new/pkg", "some:tag"},
					{"pkg", "vers"},
					{"pkg2", "vers"},
				})
			})
		})
	})

	Convey("PinSliceBySubdir", t, func() {
		id := func(letter rune) string {
			return strings.Repeat(string(letter), 40)
		}

		pmr := PinSliceBySubdir{
			"": PinSlice{
				{"pkg2", id('1')},
				{"pkg", id('0')},
			},
			"other": PinSlice{
				{"something", id('2')},
			},
		}

		Convey("Can validate", func() {
			So(pmr.Validate(), ShouldErrLike, nil)

			Convey("can see bad subdirs", func() {
				pmr["/"] = PinSlice{{"something", "version"}}
				So(pmr.Validate(), ShouldErrLike, "bad subdir")
			})

			Convey("can see duplicate packages", func() {
				pmr[""] = append(pmr[""], Pin{"pkg", strings.Repeat("2", 40)})
				So(pmr.Validate(), ShouldErrLike, `subdir "": duplicate package "pkg"`)
			})

			Convey("can see bad pins", func() {
				pmr[""] = append(pmr[""], Pin{"quxxly", "nurbs"})
				So(pmr.Validate(), ShouldErrLike, `subdir "": not a valid package instance ID`)
			})
		})

		Convey("can convert to ByMap", func() {
			pmm := pmr.ToMap()
			So(pmm, ShouldResemble, PinMapBySubdir{
				"": PinMap{
					"pkg":  id('0'),
					"pkg2": id('1'),
				},
				"other": PinMap{
					"something": id('2'),
				},
			})

			Convey("and back", func() {
				So(pmm.ToSlice(), ShouldResemble, PinSliceBySubdir{
					"": PinSlice{
						{"pkg", id('0')},
						{"pkg2", id('1')},
					},
					"other": PinSlice{
						{"something", id('2')},
					},
				})
			})
		})

	})
}

func TestCurrentResolution(t *testing.T) {
	t.Parallel()

	Convey("Sanity check on arch/os", t, func() {
		Convey("Has a known os", func() {
			known := false
			switch currentOS {
			case runtime.GOOS, "mac":
				known = true
			}
			So(known, ShouldBeTrue)
		})

		Convey("Has a known architecture", func() {
			known := false
			switch currentArchitecture {
			case runtime.GOARCH, "armv6l":
				known = true
			}
			So(known, ShouldBeTrue)
		})
	})
}
