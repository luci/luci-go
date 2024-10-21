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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/luciexe"
)

func TestSpy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("disabled on windows due to crbug.com/998936")
	}

	ftt.Run(`test build spy environment`, t, func(t *ftt.Test) {
		ctx, closer := testCtx(t)
		defer closer()

		sawBuild := false
		sawBuildC := make(chan struct{})
		defer func() {
			if !sawBuild {
				close(sawBuildC)
			}
		}()

		t.Run(`butler active within Run`, func(c *ftt.Test) {
			ch, err := Run(ctx, nil, func(ctx context.Context, _ Options, _ <-chan lucictx.DeadlineEvent, _ func()) {
				bs, _ := bootstrap.Get()

				stream, err := bs.Client.NewDatagramStream(
					ctx, luciexe.BuildProtoStreamSuffix,
					streamclient.WithContentType(luciexe.BuildProtoZlibContentType))
				assert.Loosely(c, err, should.BeNil)
				defer stream.Close()

				data, err := proto.Marshal(&bbpb.Build{
					SummaryMarkdown: "we did it",
					Status:          bbpb.Status_SUCCESS,
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_SUCCESS,
					},
				})
				assert.Loosely(c, err, should.BeNil)

				buf := bytes.Buffer{}
				z := zlib.NewWriter(&buf)
				_, err = z.Write(data)
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, z.Close(), should.BeNil)
				data = buf.Bytes()

				err = stream.WriteDatagram(data)
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, stream.Close(), should.BeNil)

				// NOTE: This is very much cheating. Currently (Sept 2019) there's a bug
				// in Logdog Butler (crbug.com/1007022) where the butler protocol is TOO
				// asynchronous, and it's possible for this entire callback to execute
				// and return before the butler has registered the stream above.
				//
				// Without sawBuildC, it's possible that Run() will close the "u/"
				// namespace before the butler sees the stream that we just opened.
				<-sawBuildC
			})

			assert.Loosely(c, err, should.BeNil)
			for build := range ch {
				if build.EndTime != nil && !sawBuild {
					build := proto.Clone(build).(*bbpb.Build)
					assert.Loosely(c, build.UpdateTime, should.NotBeNil)
					assert.Loosely(c, build.EndTime, should.NotBeNil)
					build.UpdateTime = nil
					build.EndTime = nil

					assert.Loosely(c, build, should.Resemble(&bbpb.Build{
						SummaryMarkdown: "we did it",
						Status:          bbpb.Status_SUCCESS,
						Output: &bbpb.Build_Output{
							Status: bbpb.Status_SUCCESS,
						},
					}))
					close(sawBuildC)
					sawBuild = true
				}
			}
			assert.Loosely(c, sawBuild, should.BeTrue)
		})

	})
}
