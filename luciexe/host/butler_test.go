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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/lucictx"
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
	ftt.Run(`test butler environment`, t, func(t *ftt.Test) {
		ctx, closer := testCtx(t)
		defer closer()

		t.Run(`butler active within Run`, func(c *ftt.Test) {
			ch, err := Run(ctx, nil, func(ctx context.Context, _ Options, _ <-chan lucictx.DeadlineEvent, _ func()) {
				bs, err := bootstrap.Get()
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, bs.Client, should.NotBeNil)
				assert.Loosely(c, bs.Project, should.Equal("null"))
				assert.Loosely(c, bs.Prefix, should.Equal(types.StreamName("null")))
				assert.Loosely(c, bs.Namespace, should.Equal(types.StreamName("u")))

				stream, err := bs.Client.NewStream(ctx, "sup")
				assert.Loosely(c, err, should.BeNil)
				defer stream.Close()
				_, err = stream.Write([]byte("HELLO"))
				assert.Loosely(c, err, should.BeNil)
			})
			assert.Loosely(c, err, should.BeNil)
			for range ch {
				// TODO(iannucci): check for Build object contents
			}
		})
	})
}
