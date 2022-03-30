// Copyright 2019 The LUCI Authors.
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

package host

import (
	"bytes"
	"compress/zlib"
	"context"
	"runtime"
	"testing"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSpy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("disabled on windows due to crbug.com/998936")
	}

	Convey(`test build spy environment`, t, func() {
		ctx, closer := testCtx()
		defer closer()

		sawBuild := false
		sawBuildC := make(chan struct{})
		defer func() {
			if !sawBuild {
				close(sawBuildC)
			}
		}()

		Convey(`butler active within Run`, func(c C) {
			ch, err := Run(ctx, nil, func(ctx context.Context, _ Options) error {
				bs, err := bootstrap.Get()

				stream, err := bs.Client.NewDatagramStream(
					ctx, luciexe.BuildProtoStreamSuffix,
					streamclient.WithContentType(luciexe.BuildProtoZlibContentType))
				c.So(err, ShouldBeNil)
				defer stream.Close()

				data, err := proto.Marshal(&bbpb.Build{
					SummaryMarkdown: "we did it",
					Status:          bbpb.Status_SUCCESS,
				})
				c.So(err, ShouldBeNil)

				buf := bytes.Buffer{}
				z := zlib.NewWriter(&buf)
				_, err = z.Write(data)
				c.So(err, ShouldBeNil)
				c.So(z.Close(), ShouldBeNil)
				data = buf.Bytes()

				err = stream.WriteDatagram(data)
				c.So(err, ShouldBeNil)
				c.So(stream.Close(), ShouldBeNil)

				// NOTE: This is very much cheating. Currently (Sept 2019) there's a bug
				// in Logdog Butler (crbug.com/1007022) where the butler protocol is TOO
				// asynchronous, and it's possible for this entire callback to execute
				// and return before the butler has registered the stream above.
				//
				// Without sawBuildC, it's possible that Run() will close the "u/"
				// namespace before the butler sees the stream that we just opened.
				<-sawBuildC

				return nil
			})

			So(err, ShouldBeNil)
			for build := range ch {
				if build.EndTime != nil && !sawBuild {
					build := proto.Clone(build).(*bbpb.Build)
					So(build.UpdateTime, ShouldNotBeNil)
					So(build.EndTime, ShouldNotBeNil)
					build.UpdateTime = nil
					build.EndTime = nil

					So(build, ShouldResembleProto, &bbpb.Build{
						SummaryMarkdown: "we did it",
						Status:          bbpb.Status_SUCCESS,
					})
					close(sawBuildC)
					sawBuild = true
				}
			}
			So(sawBuild, ShouldBeTrue)
		})

	})
}
