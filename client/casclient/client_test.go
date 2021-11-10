// Copyright 2021 The LUCI Authors.
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

package casclient

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIsLocalAddr(t *testing.T) {
	t.Parallel()

	Convey("isLocalAddr", t, func() {
		var testdata = []struct {
			addr string
			want bool
		}{
			{":9999", true},
			{"localhost:9999", true},
			{"127.0.0.1:9999", true},
			{"[::]:9999", false},
			{"[::1]:9999", true},
			{"remotebuildexecution.googleapis.com:443", false},
		}
		for _, td := range testdata {
			isLocal, err := isLocalAddr(td.addr)
			So(err, ShouldBeNil)
			So(isLocal, ShouldEqual, td.want)
		}
	})
}
