// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ensure

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

var fileSerializationTests = []struct {
	name   string
	f      *File
	expect string
}{
	{
		"empty",
		&File{},
		f(""),
	},

	{
		"ServiceURL",
		&File{"https://something.example.com", nil},
		f(
			"$ServiceURL https://something.example.com",
		),
	},

	{
		"simple packages",
		&File{"", map[string]PackageSlice{
			"": {
				PackageDef{"some/thing", "version", 0},
				PackageDef{"some/other_thing", "latest", 0},
			},
		}},
		f(
			"some/other_thing@latest",
			"some/thing@version",
		),
	},

	{
		"full file",
		&File{"https://some.example.com", map[string]PackageSlice{
			"": {
				PackageDef{"some/thing", "version", 0},
				PackageDef{"some/other_thing", "latest", 0},
			},
			"path/to dir/with/spaces": {
				PackageDef{"different/package", "some_tag:thingy", 0},
			},
		}},
		f(
			"$ServiceURL https://some.example.com",
			"",
			"some/other_thing@latest",
			"some/thing@version",
			"",
			"@Subdir path/to dir/with/spaces",
			"different/package@some_tag:thingy",
		),
	},
}

func TestFileSerialization(t *testing.T) {
	t.Parallel()

	Convey("File.Serialize", t, func() {
		for _, tc := range fileSerializationTests {
			Convey(tc.name, func() {
				buf := &bytes.Buffer{}
				_, err := tc.f.Serialize(buf)
				So(err, ShouldBeNil)
				So(buf.String(), ShouldEqual, tc.expect)
			})
		}
	})
}
