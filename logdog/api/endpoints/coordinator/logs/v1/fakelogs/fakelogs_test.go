// Copyright 2017 The LUCI Authors.
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

package fakelogs

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	logs "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

func TestFakeLogs(t *testing.T) {
	t.Parallel()

	Convey(`fakelogs`, t, func() {
		c := NewClient()

		Convey(`can open streams`, func() {
			st, err := c.OpenTextStream("some/prefix", "some/path", &streamproto.Flags{
				Tags: streamproto.TagMap{"tag": "value"}})
			So(err, ShouldBeNil)
			defer st.Close()

			rsp, err := c.Get(nil, &logs.GetRequest{
				Path:  "some/prefix/+/some/path",
				State: true,
			})
			So(err, ShouldBeNil)
			Println(rsp)

			sd, err := c.OpenDatagramStream("some/prefix", "other/path", &streamproto.Flags{
				ContentType: "application/json"})
			So(err, ShouldBeNil)
			defer sd.Close()

		})

	})
}
