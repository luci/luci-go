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
	"context"
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSpy(t *testing.T) {
	Convey(`test build spy environment`, t, func() {
		ctx, closer := testCtx()
		defer closer()

		Convey(`butler active within Run`, func(c C) {
			ch, err := Run(ctx, nil, func(ctx context.Context) error {
				bs, err := bootstrap.Get()

				stream, err := bs.Client.NewDatagramStream(
					ctx, luciexe.BuildProtoStreamSuffix,
					streamclient.WithContentType(luciexe.BuildProtoContentType))
				c.So(err, ShouldBeNil)
				defer stream.Close()

				data, err := proto.Marshal(&bbpb.Build{
					SummaryMarkdown: "we did it",
					Status:          bbpb.Status_SUCCESS,
				})
				c.So(err, ShouldBeNil)

				err = stream.WriteDatagram(data)
				c.So(err, ShouldBeNil)
				c.So(stream.Close(), ShouldBeNil)

				return nil
			})

			So(err, ShouldBeNil)
			for range ch {
				// TODO(iannucci): actually evaluate the builds here
			}
		})

	})
}
