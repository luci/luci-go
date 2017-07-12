// Copyright 2015 The LUCI Authors.
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

package types

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestStreamAddr(t *testing.T) {
	t.Parallel()

	var successes = []struct {
		s   string
		exp StreamAddr
	}{
		{"logdog://host/project/a/+/b", StreamAddr{"host", "project", "a/+/b"}},
		{"logdog://host.example.com/project/foo/bar/+/baz", StreamAddr{"host.example.com", "project", "foo/bar/+/baz"}},
	}

	var failures = []struct {
		s   string
		err string
	}{
		{"://project/prefix/+/name", "failed to parse URL"},
		{"http://example.com/foo/bar/+/baz", "is not logdog"},
		{"logdog://example.com/foo", "URL path does not include both project and path components"},
		{"logdog://example.com/foo@d/bar", "invalid project name"},
		{"logdog://example.com/foo/bar", "invalid stream path"},
		{"logdog://example.com/foo/bar/+/ba!", "invalid stream path"},
	}

	Convey(`Testing StreamAddr`, t, func() {

		for _, tc := range successes {
			Convey(fmt.Sprintf(`Success: %q`, tc.s), func() {
				addr, err := ParseURL(tc.s)
				So(err, ShouldBeNil)
				So(addr, ShouldResemble, &tc.exp)

				u, err := url.Parse(tc.s)
				So(err, ShouldBeNil)
				So(addr.URL(), ShouldResemble, u)
			})
		}

		for _, tc := range failures {
			Convey(fmt.Sprintf(`Failure: %q fails like: %q`, tc.s, tc.err), func() {
				_, err := ParseURL(tc.s)
				So(err, ShouldErrLike, tc.err)
			})
		}
	})

	Convey(`StreamAddr is a flag.Value`, t, func() {
		fs := flag.NewFlagSet("testing", flag.ContinueOnError)
		a := &StreamAddr{}

		fs.Var(a, "addr", "its totally an address of a thing")

		Convey(`good`, func() {
			So(fs.Parse([]string{"-addr", "logdog://host/project/a/+/b"}), ShouldBeNil)
			So(a, ShouldResemble, &StreamAddr{
				"host",
				"project",
				"a/+/b",
			})
		})

		Convey(`bad`, func() {
			So(fs.Parse([]string{"-addr", "://host/project/a/+/b"}), ShouldErrLike,
				"failed to parse URL")
		})
	})

	Convey(`StreamAddr as a json value`, t, func() {
		a := &StreamAddr{}

		Convey(`good`, func() {
			Convey(`zero`, func() {
				data, err := json.Marshal(a)
				So(err, ShouldBeNil)
				So(string(data), ShouldResemble, `{}`)
				So(json.Unmarshal(data, a), ShouldBeNil)
				So(a, ShouldResemble, &StreamAddr{})
			})

			Convey(`full`, func() {
				a.Host = "host"
				a.Project = "project"
				a.Path = "a/+/b"
				data, err := json.Marshal(a)
				So(err, ShouldBeNil)
				So(string(data), ShouldResemble, `{"host":"host","project":"project","path":"a/+/b"}`)

				a2 := &StreamAddr{}
				So(json.Unmarshal(data, a2), ShouldBeNil)
				So(a2, ShouldResemble, a)
			})
		})

		Convey(`bad`, func() {
			So(json.Unmarshal([]byte(`{"host":"host","project":"project","path":"fake"}`), a), ShouldErrLike,
				"must contain at least one character") // from bad Path
		})
	})
}
