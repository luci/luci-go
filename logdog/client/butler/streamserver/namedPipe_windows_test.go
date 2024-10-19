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

package streamserver

import (
	"context"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

func TestWindowsNamedPipeServer(t *testing.T) {
	t.Skip("test disabled on windows for flake: crbug.com/998936")

	t.Parallel()

	ftt.Run(`A named pipe server`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`Will generate a prefix if none is provided.`, func(t *ftt.Test) {
			srv, err := newStreamServer(ctx, "")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, srv.Address(), should.HavePrefix("net.pipe:"+defaultWinPipePrefix))
		})

		t.Run(`Will refuse to create if longer than maximum length.`, func(t *ftt.Test) {
			_, err := newStreamServer(ctx, strings.Repeat("A", maxWindowsNamedPipeLength+1))
			assert.Loosely(t, err, should.ErrLike("path exceeds maximum length"))
		})

		t.Run(`When created and listening.`, func(t *ftt.Test) {
			svr, err := newStreamServer(ctx, "ButlerNamedPipeTest")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, svr.Listen(), should.BeNil)
			defer svr.Close()

			client, err := streamclient.New(svr.Address(), "")
			assert.Loosely(t, err, should.BeNil)

			testClientServer(t, svr, client)
		})
	})
}
