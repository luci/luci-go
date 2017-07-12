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

func TestCheckURL(t *testing.T) {
	Convey(`Verifies that CheckURL properly checks a URL.`, t, func() {
		data := []struct {
			in       string
			expected string
			err      error
		}{
			{"foo", "https://foo", nil},
			{"https://foo", "https://foo", nil},
			{"http://foo.example.com", "http://foo.example.com", nil},
			{"http://foo.appspot.com", "", errors.New("only https:// scheme is accepted for appspot hosts, it can be omitted")},
		}
		for _, line := range data {
			out, err := CheckURL(line.in)
			So(out, ShouldResemble, line.expected)
			So(err, ShouldResemble, line.err)
		}
	})
}
