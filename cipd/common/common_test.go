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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

func TestValidatePackageName(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidatePackageName works", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidatePackageName("good/name"), should.BeNil)
		assert.Loosely(t, ValidatePackageName("good_name"), should.BeNil)
		assert.Loosely(t, ValidatePackageName("123-_/also/good/name"), should.BeNil)
		assert.Loosely(t, ValidatePackageName("good.name/.name/..name"), should.BeNil)
		assert.Loosely(t, ValidatePackageName(""), should.NotBeNil)
		assert.Loosely(t, ValidatePackageName("/"), should.NotBeNil)
		assert.Loosely(t, ValidatePackageName("BAD/name"), should.NotBeNil)
		assert.Loosely(t, ValidatePackageName("bad//name"), should.NotBeNil)
		assert.Loosely(t, ValidatePackageName("bad/name/"), should.NotBeNil)
		assert.Loosely(t, ValidatePackageName("/bad/name"), should.NotBeNil)
		assert.Loosely(t, ValidatePackageName("bad/name\nyeah"), should.NotBeNil)
		assert.Loosely(t, ValidatePackageName("./name"), should.NotBeNil)
		assert.Loosely(t, ValidatePackageName("name/../name"), should.NotBeNil)
		assert.Loosely(t, ValidatePackageName("../../yeah"), should.NotBeNil)
		assert.Loosely(t, ValidatePackageName("..."), should.NotBeNil)
	})
}

func TestValidatePackagePrefix(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidatePackagePrefix strips suffix", t, func(t *ftt.Test) {
		p, err := ValidatePackagePrefix("good/name/")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, p, should.Equal("good/name"))
	})

	ftt.Run("ValidatePackagePrefix works", t, func(t *ftt.Test) {
		call := func(p string) error {
			_, err := ValidatePackagePrefix(p)
			return err
		}

		assert.Loosely(t, call("good/name"), should.BeNil)
		assert.Loosely(t, call("good/name/"), should.BeNil)
		assert.Loosely(t, call("good_name"), should.BeNil)
		assert.Loosely(t, call("123-_/also/good/name"), should.BeNil)
		assert.Loosely(t, call("good.name/.name/..name"), should.BeNil)
		assert.Loosely(t, call(""), should.BeNil)  // repo root
		assert.Loosely(t, call("/"), should.BeNil) // repo root
		assert.Loosely(t, call("BAD/name"), should.NotBeNil)
		assert.Loosely(t, call("bad//name"), should.NotBeNil)
		assert.Loosely(t, call("bad/name//"), should.NotBeNil)
		assert.Loosely(t, call("/bad/name"), should.NotBeNil)
		assert.Loosely(t, call("bad/name\nyeah"), should.NotBeNil)
		assert.Loosely(t, call("./name"), should.NotBeNil)
		assert.Loosely(t, call("name/../name"), should.NotBeNil)
		assert.Loosely(t, call("../../yeah"), should.NotBeNil)
		assert.Loosely(t, call("..."), should.NotBeNil)
	})
}

func TestValidateInstanceTag(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateInstanceTag works", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateInstanceTag(""), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceTag("notapair"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceTag(strings.Repeat("long", 200)+":abc"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceTag("BADKEY:value"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceTag("empty_val:"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceTag(" space:a"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceTag("space :a"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceTag("space: a"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceTag("space:a "), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceTag("newline:a\n"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceTag("tab:a\tb"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceTag("good:tag"), should.BeNil)
		assert.Loosely(t, ValidateInstanceTag("good:tag:blah"), should.BeNil)
		assert.Loosely(t, ValidateInstanceTag("good_tag:A a0$()*+,-./:;<=>@\\_{}~"), should.BeNil)
	})
}

func TestParseInstanceTag(t *testing.T) {
	t.Parallel()

	ftt.Run("ParseInstanceTag works", t, func(t *ftt.Test) {
		pTag, err := ParseInstanceTag("good:tag")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pTag, should.Match(&api.Tag{
			Key:   "good",
			Value: "tag",
		}))

		pTag, err = ParseInstanceTag("good:tag:blah")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pTag, should.Match(&api.Tag{
			Key:   "good",
			Value: "tag:blah",
		}))

		pTag, err = ParseInstanceTag("good_tag:A a0$()*+,-./:;<=>@\\_{}~")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pTag, should.Match(&api.Tag{
			Key:   "good_tag",
			Value: "A a0$()*+,-./:;<=>@\\_{}~",
		}))

		pTag, err = ParseInstanceTag("")
		assert.Loosely(t, err, should.NotBeNil)

		pTag, err = ParseInstanceTag("notapair")
		assert.Loosely(t, err, should.NotBeNil)

		pTag, err = ParseInstanceTag(strings.Repeat("long", 200) + ":abc")
		assert.Loosely(t, err, should.NotBeNil)

		pTag, err = ParseInstanceTag("BADKEY:value")
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("MustParseInstanceTag panics on bad tag", t, func(t *ftt.Test) {
		assert.Loosely(t, func() { MustParseInstanceTag("") }, should.Panic)
	})
}

func TestValidatePin(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidatePin works", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidatePin(Pin{"good/name", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, KnownHash), should.BeNil)
		assert.Loosely(t, ValidatePin(Pin{"BAD/name", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, KnownHash), should.NotBeNil)
		assert.Loosely(t, ValidatePin(Pin{"good/name", "aaaaaaaaaaa"}, KnownHash), should.NotBeNil)
	})
}

func TestValidatePackageRef(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidatePackageRef works", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidatePackageRef("some-ref"), should.BeNil)
		assert.Loosely(t, ValidatePackageRef("ref/with/slashes.and.dots"), should.BeNil)

		assert.Loosely(t, ValidatePackageRef(""), should.NotBeNil)
		assert.Loosely(t, ValidatePackageRef("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), should.NotBeNil)
		assert.Loosely(t, ValidatePackageRef("good:tag"), should.NotBeNil)
	})
}

func TestValidateInstanceVersion(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateInstanceVersion works", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateInstanceVersion("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), should.BeNil)
		assert.Loosely(t, ValidateInstanceVersion("good:tag"), should.BeNil)
		assert.Loosely(t, ValidatePackageRef("some-read"), should.BeNil)
		assert.Loosely(t, ValidateInstanceVersion("BADTAG:"), should.NotBeNil)
	})
}

func TestValidateSubdir(t *testing.T) {
	t.Parallel()

	badSubdirs := []struct {
		name   string
		subdir string
		err    string
	}{
		{"windows", "folder\\thing", "backslashes are not allowed"},
		{"windows drive", "c:/foo/bar", `colons are not allowed`},
		{"messy", "some/../thing", `"some/../thing": should be simplified to "thing"`},
		{"relative", "../something", `contains disallowed dot-path prefix`},
		{"single relative", "./something", `"./something": should be simplified to "something"`},
		{"absolute", "/etc", `absolute paths are not allowed`},
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

	ftt.Run("ValidtateSubdir", t, func(t *ftt.Test) {
		t.Run("rejects bad subdirs", func(t *ftt.Test) {
			for _, tc := range badSubdirs {
				t.Run(tc.name, func(t *ftt.Test) {
					assert.Loosely(t, ValidateSubdir(tc.subdir), should.ErrLike(tc.err))
				})
			}
		})

		t.Run("accepts good subdirs", func(t *ftt.Test) {
			for _, tc := range goodSubdirs {
				t.Run(tc.name, func(t *ftt.Test) {
					assert.NoErr(t, ValidateSubdir(tc.subdir))
				})
			}
		})
	})
}

func TestValidatePrincipalName(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidatePrincipalName OK", t, func(t *ftt.Test) {
		cases := []string{
			"group:abc",
			"user:a@example.com",
			"anonymous:anonymous",
			"bot:blah",
			"service:blah",
		}
		for _, tc := range cases {
			assert.Loosely(t, ValidatePrincipalName(tc), should.BeNil)
		}
	})

	ftt.Run("ValidatePrincipalName not OK", t, func(t *ftt.Test) {
		cases := []struct{ p, err string }{
			{"", "doesn't look like a principal id"},
			{":", "doesn't look like a principal id"},
			{":zzz", "doesn't look like a principal id"},
			{"group:", "doesn't look like a principal id"},
			{"user:", "doesn't look like a principal id"},
			{"anonymous:zzz", `bad value "zzz" for identity kind "anonymous"`},
			{"user:abc", `bad value "abc" for identity kind "user"`},
		}
		for _, tc := range cases {
			assert.Loosely(t, ValidatePrincipalName(tc.p), should.ErrLike(tc.err))
		}
	})
}

func TestNormalizePrefixMetadata(t *testing.T) {
	t.Parallel()

	ftt.Run("Happy path", t, func(t *ftt.Test) {
		m := &api.PrefixMetadata{
			Prefix: "abc/",
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_OWNER, Principals: []string{"user:abc@example.com", "group:a"}},
				{Role: api.Role_READER, Principals: []string{"group:z"}},
				{Role: 123}, // some future unknown role
			},
		}
		assert.Loosely(t, NormalizePrefixMetadata(m), should.BeNil)
		assert.Loosely(t, m, should.Match(&api.PrefixMetadata{
			Prefix: "abc",
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{"group:z"}},
				{Role: api.Role_OWNER, Principals: []string{"group:a", "user:abc@example.com"}},
				{Role: 123},
			},
		}))
	})

	ftt.Run("Validates prefix", t, func(t *ftt.Test) {
		assert.Loosely(t, NormalizePrefixMetadata(&api.PrefixMetadata{Prefix: "//"}),
			should.ErrLike("invalid package prefix"))
	})

	ftt.Run("No role", t, func(t *ftt.Test) {
		assert.Loosely(t, NormalizePrefixMetadata(&api.PrefixMetadata{
			Prefix: "abc",
			Acls: []*api.PrefixMetadata_ACL{
				{},
			},
		}), should.ErrLike("ACL entry #0 doesn't have a role specified"))
	})

	ftt.Run("Double ACL entries", t, func(t *ftt.Test) {
		assert.Loosely(t, NormalizePrefixMetadata(&api.PrefixMetadata{
			Prefix: "abc",
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER},
				{Role: api.Role_READER},
			},
		}), should.ErrLike("role READER is specified twice"))
	})

	ftt.Run("Bad principal", t, func(t *ftt.Test) {
		assert.Loosely(t, NormalizePrefixMetadata(&api.PrefixMetadata{
			Prefix: "abc",
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{":"}},
			},
		}), should.ErrLike(`in ACL entry for role READER: ":" doesn't look like a principal id`))
	})
}

func TestPinToString(t *testing.T) {
	t.Parallel()

	ftt.Run("Pin.String works", t, func(t *ftt.Test) {
		assert.Loosely(t,
			fmt.Sprintf("%s", Pin{"good/name", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}),
			should.Equal(
				"good/name:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	})
}

func TestPinSliceAndMap(t *testing.T) {
	t.Parallel()

	ftt.Run("PinSlice", t, func(t *ftt.Test) {
		ps := PinSlice{{"pkg2", "vers"}, {"pkg", "vers"}}

		t.Run("can convert to a map", func(t *ftt.Test) {
			pm := ps.ToMap()
			assert.Loosely(t, pm, should.Match(PinMap{
				"pkg":  "vers",
				"pkg2": "vers",
			}))

			pm["new/pkg"] = "some:tag"

			t.Run("and back to a slice", func(t *ftt.Test) {
				assert.Loosely(t, pm.ToSlice(), should.Match(PinSlice{
					{"new/pkg", "some:tag"},
					{"pkg", "vers"},
					{"pkg2", "vers"},
				}))
			})
		})
	})

	ftt.Run("PinSliceBySubdir", t, func(t *ftt.Test) {
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

		t.Run("Can validate", func(t *ftt.Test) {
			assert.NoErr(t, pmr.Validate(AnyHash))

			t.Run("can see bad subdirs", func(t *ftt.Test) {
				pmr["/"] = PinSlice{{"something", "version"}}
				assert.Loosely(t, pmr.Validate(AnyHash), should.ErrLike("bad subdir"))
			})

			t.Run("can see duplicate packages", func(t *ftt.Test) {
				pmr[""] = append(pmr[""], Pin{"pkg", strings.Repeat("2", 40)})
				assert.Loosely(t, pmr.Validate(AnyHash), should.ErrLike(`subdir "": duplicate package "pkg"`))
			})

			t.Run("can see bad pins", func(t *ftt.Test) {
				pmr[""] = append(pmr[""], Pin{"quxxly", "nurbs"})
				assert.Loosely(t, pmr.Validate(AnyHash), should.ErrLike(`subdir "": not a valid package instance ID`))
			})
		})

		t.Run("can convert to ByMap", func(t *ftt.Test) {
			pmm := pmr.ToMap()
			assert.Loosely(t, pmm, should.Match(PinMapBySubdir{
				"": PinMap{
					"pkg":  id('0'),
					"pkg2": id('1'),
				},
				"other": PinMap{
					"something": id('2'),
				},
			}))

			t.Run("and back", func(t *ftt.Test) {
				assert.Loosely(t, pmm.ToSlice(), should.Match(PinSliceBySubdir{
					"": PinSlice{
						{"pkg", id('0')},
						{"pkg2", id('1')},
					},
					"other": PinSlice{
						{"something", id('2')},
					},
				}))
			})
		})
	})
}

func TestInstanceMetadata(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateInstanceMetadataKey works", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateInstanceMetadataKey("a"), should.BeNil)
		assert.Loosely(t, ValidateInstanceMetadataKey("az_-09"), should.BeNil)
		assert.Loosely(t, ValidateInstanceMetadataKey(strings.Repeat("z", 400)), should.BeNil)

		assert.Loosely(t, ValidateInstanceMetadataKey(""), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceMetadataKey(strings.Repeat("z", 401)), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceMetadataKey("a a"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceMetadataKey("A"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceMetadataKey("a:a"), should.NotBeNil)
	})

	ftt.Run("ValidateContentType works", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateContentType(""), should.BeNil)
		assert.Loosely(t, ValidateContentType("text/plain; encoding=utf-8"), should.BeNil)

		assert.Loosely(t, ValidateContentType("zzz zzz"), should.NotBeNil)
		assert.Loosely(t, ValidateContentType(strings.Repeat("z", 401)), should.NotBeNil)
	})

	ftt.Run("InstanceMetadataFingerprint works", t, func(t *ftt.Test) {
		fp := InstanceMetadataFingerprint("key", []byte("value"))
		assert.Loosely(t, fp, should.Equal("06dd3884aa86b22603764bd7e5b0b41e"))
	})

	ftt.Run("ValidateInstanceMetadataFingerprint works", t, func(t *ftt.Test) {
		fp := InstanceMetadataFingerprint("key", []byte("value"))
		assert.Loosely(t, ValidateInstanceMetadataFingerprint(fp), should.BeNil)
		assert.Loosely(t, ValidateInstanceMetadataFingerprint("aaaa"), should.NotBeNil)
		assert.Loosely(t, ValidateInstanceMetadataFingerprint(strings.Repeat("Z", 32)), should.NotBeNil)
	})
}
