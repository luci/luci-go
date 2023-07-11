// Copyright 2020 The LUCI Authors.
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

package ledcli

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
)

func TestParseCLURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		url             string
		err             string
		cl              *bbpb.GerritChange
		resolvePatchset int64
	}{
		{
			url: "",
			err: "only *-review.googlesource.com URLs are supported",
		},

		{
			url: "%20://",
			err: "URL_TO_CHANGELIST: parse",
		},

		{
			url: "https://other.domain.example.com/stuff/things",
			err: "only *-review.googlesource.com URLs are supported",
		},

		{
			url: "https://thing-review.googlesource.com/",
			err: "old/empty",
		},

		{
			url: "https://thing-review.googlesource.com/#/c/oldstyle",
			err: "old/empty",
		},

		{
			url: "https://thing-review.googlesource.com/wat",
			err: "gerrit URL parsing change",
		},

		{
			url: "https://thing-review.googlesource.com/c/+/1235",
			err: "missing project",
		},

		{
			url: "https://thing-review.googlesource.com/c/project/+",
			err: "missing change/patchset",
		},

		{
			url: "https://thing-review.googlesource.com/c/project/+/nan",
			err: "parsing change",
		},

		{
			url: "https://thing-review.googlesource.com/c/project/+/123/nan",
			err: "parsing patchset",
		},

		{
			url: "https://thing-review.googlesource.com/c/project/+/1111",
			err: "TEST: resolvePatchset not set",
		},

		{
			url: "https://thing-review.googlesource.com/c/project/+/123",

			resolvePatchset: 1024,
			cl: &bbpb.GerritChange{
				Host:     "thing-review.googlesource.com",
				Project:  "project",
				Change:   123,
				Patchset: 1024,
			},
		},

		{
			url: "https://thing-review.googlesource.com/c/project/+/123/1337",
			cl: &bbpb.GerritChange{
				Host:     "thing-review.googlesource.com",
				Project:  "project",
				Change:   123,
				Patchset: 1337,
			},
		},
	}

	Convey(`parseCrChangeListURL`, t, func() {
		for _, tc := range cases {
			tc := tc
			Convey(fmt.Sprintf("%q", tc.url), func() {
				cl, err := parseCrChangeListURL(tc.url, func(string, int64) (string, int64, error) {
					if tc.resolvePatchset != 0 {
						return "project", tc.resolvePatchset, nil
					}
					return "", 0, errors.New("TEST: resolvePatchset not set")
				})
				if tc.err != "" {
					So(err, ShouldErrLike, tc.err)
					So(cl, ShouldBeNil)
				} else {
					So(err, ShouldBeNil)
					So(cl, ShouldResembleProto, tc.cl)
				}
			})
		}

	})
}
