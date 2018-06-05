// Copyright 2014 The LUCI Authors.
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

package common

import (
	"fmt"
	"strings"
	"testing"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidatePackageName(t *testing.T) {
	t.Parallel()

	Convey("ValidatePackageName works", t, func() {
		So(ValidatePackageName("good/name"), ShouldBeNil)
		So(ValidatePackageName("good_name"), ShouldBeNil)
		So(ValidatePackageName("123-_/also/good/name"), ShouldBeNil)
		So(ValidatePackageName("good.name/.name/..name"), ShouldBeNil)
		So(ValidatePackageName(""), ShouldNotBeNil)
		So(ValidatePackageName("/"), ShouldNotBeNil)
		So(ValidatePackageName("BAD/name"), ShouldNotBeNil)
		So(ValidatePackageName("bad//name"), ShouldNotBeNil)
		So(ValidatePackageName("bad/name/"), ShouldNotBeNil)
		So(ValidatePackageName("/bad/name"), ShouldNotBeNil)
		So(ValidatePackageName("bad/name\nyeah"), ShouldNotBeNil)
		So(ValidatePackageName("./name"), ShouldNotBeNil)
		So(ValidatePackageName("name/../name"), ShouldNotBeNil)
		So(ValidatePackageName("../../yeah"), ShouldNotBeNil)
		So(ValidatePackageName("..."), ShouldNotBeNil)
	})
}

func TestValidatePackagePrefix(t *testing.T) {
	t.Parallel()

	Convey("ValidatePackagePrefix strips suffix", t, func() {
		p, err := ValidatePackagePrefix("good/name/")
		So(err, ShouldBeNil)
		So(p, ShouldEqual, "good/name")
	})

	Convey("ValidatePackagePrefix works", t, func() {
		call := func(p string) error {
			_, err := ValidatePackagePrefix(p)
			return err
		}

		So(call("good/name"), ShouldBeNil)
		So(call("good/name/"), ShouldBeNil)
		So(call("good_name"), ShouldBeNil)
		So(call("123-_/also/good/name"), ShouldBeNil)
		So(call("good.name/.name/..name"), ShouldBeNil)
		So(call(""), ShouldBeNil)  // repo root
		So(call("/"), ShouldBeNil) // repo root
		So(call("BAD/name"), ShouldNotBeNil)
		So(call("bad//name"), ShouldNotBeNil)
		So(call("bad/name//"), ShouldNotBeNil)
		So(call("/bad/name"), ShouldNotBeNil)
		So(call("bad/name\nyeah"), ShouldNotBeNil)
		So(call("./name"), ShouldNotBeNil)
		So(call("name/../name"), ShouldNotBeNil)
		So(call("../../yeah"), ShouldNotBeNil)
		So(call("..."), ShouldNotBeNil)
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
		So(ValidatePackageRef("ref/with/slashes.and.dots"), ShouldBeNil)

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
	t.Parallel()

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

func TestValidatePrincipalName(t *testing.T) {
	t.Parallel()

	Convey("ValidatePrincipalName OK", t, func() {
		cases := []string{
			"group:abc",
			"user:a@example.com",
			"anonymous:anonymous",
			"bot:blah",
			"service:blah",
		}
		for _, tc := range cases {
			So(ValidatePrincipalName(tc), ShouldBeNil)
		}
	})

	Convey("ValidatePrincipalName not OK", t, func() {
		cases := []struct{ p, err string }{
			{"", "doesn't look like principal id"},
			{":", "doesn't look like principal id"},
			{":zzz", "doesn't look like principal id"},
			{"group:", "doesn't look like principal id"},
			{"user:", "doesn't look like principal id"},
			{"anonymous:zzz", `bad value "zzz" for identity kind "anonymous"`},
			{"user:abc", `bad value "abc" for identity kind "user"`},
		}
		for _, tc := range cases {
			So(ValidatePrincipalName(tc.p), ShouldErrLike, tc.err)
		}
	})
}

func TestNormalizePrefixMetadata(t *testing.T) {
	t.Parallel()

	Convey("Happy path", t, func() {
		m := &api.PrefixMetadata{
			Prefix: "abc/",
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_OWNER, Principals: []string{"user:abc@example.com", "group:a"}},
				{Role: api.Role_READER, Principals: []string{"group:z"}},
				{Role: 123}, // some future unknown role
			},
		}
		So(NormalizePrefixMetadata(m), ShouldBeNil)
		So(m, ShouldResemble, &api.PrefixMetadata{
			Prefix: "abc",
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{"group:z"}},
				{Role: api.Role_OWNER, Principals: []string{"group:a", "user:abc@example.com"}},
				{Role: 123},
			},
		})
	})

	Convey("Validates prefix", t, func() {
		So(NormalizePrefixMetadata(&api.PrefixMetadata{Prefix: "//"}),
			ShouldErrLike, "invalid package prefix")
	})

	Convey("No role", t, func() {
		So(NormalizePrefixMetadata(&api.PrefixMetadata{
			Prefix: "abc",
			Acls: []*api.PrefixMetadata_ACL{
				{},
			},
		}), ShouldErrLike, "ACL entry #0 doesn't have a role specified")
	})

	Convey("Double ACL entries", t, func() {
		So(NormalizePrefixMetadata(&api.PrefixMetadata{
			Prefix: "abc",
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER},
				{Role: api.Role_READER},
			},
		}), ShouldErrLike, "role READER is specified twice")
	})

	Convey("Bad principal", t, func() {
		So(NormalizePrefixMetadata(&api.PrefixMetadata{
			Prefix: "abc",
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{":"}},
			},
		}), ShouldErrLike, `in ACL entry for role READER - ":" doesn't look like principal id`)
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
