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

package lhttp

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseHostURL(t *testing.T) {
	Convey(`Verifies that ParseHostURL properly checks a URL.`, t, func() {
		data := []struct {
			in     string
			scheme string
			host   string
			err    error
		}{
			{"foo/zzz", "https", "foo", nil},
			{"https://foo/zzz?a=b#zzz", "https", "foo", nil},
			{"http://localhost:1111", "http", "localhost:1111", nil},
			{"https://foo.appspot.com/", "https", "foo.appspot.com", nil},
			{"http://foo.appspot.com", "", "", errors.New("http:// can only be used with localhost servers")},
		}
		for _, line := range data {
			out, err := ParseHostURL(line.in)
			So(err, ShouldResemble, line.err)
			if line.err == nil {
				So(out, ShouldNotBeNil)
				So(out.Scheme, ShouldEqual, line.scheme)
				So(out.Host, ShouldEqual, line.host)
				So(out.Path, ShouldEqual, "")
			}
		}
	})
}
