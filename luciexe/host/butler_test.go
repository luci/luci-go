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
	"os"
	"strings"
	"testing"

	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/lucictx"

	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	bufferLogs = false

	for k := range environ.System().Map() {
		if strings.HasPrefix(k, "LOGDOG_") {
			os.Unsetenv(k)
		}
	}
}

func TestButler(t *testing.T) {
	Convey(`test butler environment`, t, func() {
		ctx, closer := testCtx()
		defer closer()

		Convey(`butler active within Run`, func(c C) {
			ch, err := Run(ctx, nil, func(ctx context.Context, _ Options, _ <-chan lucictx.DeadlineEvent, _ func()) {
				bs, err := bootstrap.Get()
				c.So(err, ShouldBeNil)
				c.So(bs.Client, ShouldNotBeNil)
				c.So(bs.Project, ShouldEqual, "null")
				c.So(bs.Prefix, ShouldEqual, "null")
				c.So(bs.Namespace, ShouldEqual, "u")

				stream, err := bs.Client.NewStream(ctx, "sup")
				c.So(err, ShouldBeNil)
				defer stream.Close()
				_, err = stream.Write([]byte("HELLO"))
				c.So(err, ShouldBeNil)
			})
			So(err, ShouldBeNil)
			for range ch {
				// TODO(iannucci): check for Build object contents
			}
		})
	})
}
