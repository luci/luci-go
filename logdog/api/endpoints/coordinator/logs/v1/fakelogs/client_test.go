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
	"context"
	"fmt"
	"io"
	"testing"

	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	logs "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/fetcher"
)

func TestFakeLogs(t *testing.T) {
	t.Parallel()

	ftt.Run(`fakelogs`, t, func(t *ftt.Test) {
		c := NewClient()
		ctx := gologger.StdConfig.Use(context.Background())

		t.Run(`can open streams`, func(t *ftt.Test) {
			st, err := c.OpenTextStream("some/prefix", "some/path", &streamproto.Flags{
				Tags: streamproto.TagMap{"tag": "value"}})
			assert.Loosely(t, err, should.BeNil)
			defer st.Close()

			_, err = c.Get(ctx, &logs.GetRequest{
				Path:  "some/prefix/+/some/path",
				State: true,
			})
			assert.Loosely(t, err, should.BeNil)

			sd, err := c.OpenDatagramStream("some/prefix", "other/path", &streamproto.Flags{
				ContentType: "application/json"})
			assert.Loosely(t, err, should.BeNil)
			defer sd.Close()

			t.Run(`can't open streams twice`, func(t *ftt.Test) {
				_, err := c.OpenTextStream("some/prefix", "some/path", &streamproto.Flags{
					Tags: streamproto.TagMap{"tag": "value"}})
				assert.Loosely(t, err, should.ErrLike(`duplicate stream`))
			})

			t.Run(`can query`, func(t *ftt.Test) {
				rsp, err := c.Query(ctx, &logs.QueryRequest{
					Project: Project,
					Path:    "some/prefix/+/**",
					Tags: map[string]string{
						"tag": "",
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp, should.Resemble(&logs.QueryResponse{
					Project: Project,
					Realm:   Realm,
					Streams: []*logs.QueryResponse_Stream{
						{Path: "some/prefix/+/some/path"},
					},
				}))

				rsp, err = c.Query(ctx, &logs.QueryRequest{
					Path: "some/prefix/+/other/**",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp, should.Resemble(&logs.QueryResponse{
					Project: Project,
					Realm:   Realm,
					Streams: []*logs.QueryResponse_Stream{
						{Path: "some/prefix/+/other/path"},
					},
				}))
			})
		})

		t.Run(`can write text streams`, func(t *ftt.Test) {
			st, err := c.OpenTextStream("some/prefix", "some/path")
			assert.Loosely(t, err, should.BeNil)

			fmt.Fprintf(st, "I am a banana")
			fmt.Fprintf(st, "this is\ntwo lines")
			assert.Loosely(t, st.Close(), should.BeNil)

			client := &coordinator.Client{C: c, Host: "testing-host.example.com"}
			stream := client.Stream(Project, "some/prefix/+/some/path")
			data, err := io.ReadAll(stream.Fetcher(ctx, &fetcher.Options{
				RequireCompleteStream: true,
			}).Reader())
			assert.Loosely(t, err, should.ErrLike(nil))

			assert.Loosely(t, string(data), should.Match("I am a banana\nthis is\ntwo lines\n"))
		})

		t.Run(`can write datagram streams`, func(t *ftt.Test) {
			st, err := c.OpenDatagramStream("some/prefix", "some/path")
			assert.Loosely(t, err, should.BeNil)

			fmt.Fprintf(st, "I am a banana")
			fmt.Fprintf(st, "this is\ntwo lines")
			assert.Loosely(t, st.Close(), should.BeNil)

			client := &coordinator.Client{C: c, Host: "testing-host.example.com"}
			stream := client.Stream(Project, "some/prefix/+/some/path")
			ent, err := stream.Tail(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(ent.GetDatagram().Data), should.Match("this is\ntwo lines"))
		})

		t.Run(`can write binary streams`, func(t *ftt.Test) {
			st, err := c.OpenBinaryStream("some/prefix", "some/path")
			assert.Loosely(t, err, should.BeNil)

			fmt.Fprintf(st, "I am a banana")
			fmt.Fprintf(st, "this is\ntwo lines")
			assert.Loosely(t, st.Close(), should.BeNil)

			client := &coordinator.Client{C: c, Host: "testing-host.example.com"}
			stream := client.Stream(Project, "some/prefix/+/some/path")
			data, err := io.ReadAll(stream.Fetcher(ctx, &fetcher.Options{
				RequireCompleteStream: true,
			}).Reader())
			assert.Loosely(t, err, should.ErrLike(nil))

			assert.Loosely(t, data, should.Resemble([]byte("I am a bananathis is\ntwo lines")))
		})

	})
}
