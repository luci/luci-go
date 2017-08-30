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
	"fmt"
	"io/ioutil"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/logging/gologger"
	. "go.chromium.org/luci/common/testing/assertions"
	logs "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/fetcher"
)

func TestFakeLogs(t *testing.T) {
	t.Parallel()

	Convey(`fakelogs`, t, func() {
		c := NewClient()
		ctx := gologger.StdConfig.Use(context.Background())

		Convey(`can open streams`, func() {
			st, err := c.OpenTextStream("some/prefix", "some/path", &streamproto.Flags{
				Tags: streamproto.TagMap{"tag": "value"}})
			So(err, ShouldBeNil)
			defer st.Close()

			_, err = c.Get(ctx, &logs.GetRequest{
				Path:  "some/prefix/+/some/path",
				State: true,
			})
			So(err, ShouldBeNil)

			sd, err := c.OpenDatagramStream("some/prefix", "other/path", &streamproto.Flags{
				ContentType: "application/json"})
			So(err, ShouldBeNil)
			defer sd.Close()

			Convey(`can't open streams twice`, func() {
				_, err := c.OpenTextStream("some/prefix", "some/path", &streamproto.Flags{
					Tags: streamproto.TagMap{"tag": "value"}})
				So(err, ShouldErrLike, `duplicate stream`)
			})

			Convey(`can query`, func() {
				rsp, err := c.Query(ctx, &logs.QueryRequest{
					Project: coordinatorTest.AllAccessProject,
					Tags: map[string]string{
						"tag": "",
					},
				})
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &logs.QueryResponse{
					Streams: []*logs.QueryResponse_Stream{
						{Path: "some/prefix/+/some/path"},
					},
				})

				rsp, err = c.Query(ctx, &logs.QueryRequest{
					Path: "**/+/other/**",
				})
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &logs.QueryResponse{
					Streams: []*logs.QueryResponse_Stream{
						{Path: "some/prefix/+/other/path"},
					},
				})
			})
		})

		Convey(`can write text streams`, func() {
			st, err := c.OpenTextStream("some/prefix", "some/path")
			So(err, ShouldBeNil)

			fmt.Fprintf(st, "I am a banana")
			fmt.Fprintf(st, "this is\ntwo lines")
			So(st.Close(), ShouldBeNil)

			client := &coordinator.Client{C: c, Host: "testing-host.example.com"}
			stream := client.Stream(coordinatorTest.AllAccessProject, "some/prefix/+/some/path")
			data, err := ioutil.ReadAll(stream.Fetcher(ctx, &fetcher.Options{
				RequireCompleteStream: true,
			}).Reader())
			So(err, ShouldErrLike, nil)

			So(string(data), ShouldResemble, "I am a banana\nthis is\ntwo lines\n")
		})

		Convey(`can write datagram streams`, func() {
			st, err := c.OpenDatagramStream("some/prefix", "some/path")
			So(err, ShouldBeNil)

			fmt.Fprintf(st, "I am a banana")
			fmt.Fprintf(st, "this is\ntwo lines")
			So(st.Close(), ShouldBeNil)

			client := &coordinator.Client{C: c, Host: "testing-host.example.com"}
			stream := client.Stream(coordinatorTest.AllAccessProject, "some/prefix/+/some/path")
			ent, err := stream.Tail(ctx)
			So(err, ShouldBeNil)
			So(string(ent.GetDatagram().Data), ShouldResemble, "this is\ntwo lines")
		})

		Convey(`can write binary streams`, func() {
			st, err := c.OpenBinaryStream("some/prefix", "some/path")
			So(err, ShouldBeNil)

			fmt.Fprintf(st, "I am a banana")
			fmt.Fprintf(st, "this is\ntwo lines")
			So(st.Close(), ShouldBeNil)

			client := &coordinator.Client{C: c, Host: "testing-host.example.com"}
			stream := client.Stream(coordinatorTest.AllAccessProject, "some/prefix/+/some/path")
			data, err := ioutil.ReadAll(stream.Fetcher(ctx, &fetcher.Options{
				RequireCompleteStream: true,
			}).Reader())
			So(err, ShouldErrLike, nil)

			So(data, ShouldResemble, []byte("I am a bananathis is\ntwo lines"))
		})

	})
}
