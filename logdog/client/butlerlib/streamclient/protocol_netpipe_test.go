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

//go:build windows
// +build windows

package streamclient

import (
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/Microsoft/go-winio"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

func TestNamedPipe(t *testing.T) {
	// TODO(crbug.com/998936): fix this test.
	t.Skip()

	t.Parallel()

	counter := 0

	ftt.Run(`test windows NamedPipe`, t, func(t *ftt.Test) {
		defer timebomb()()

		ctx, cancel := mkTestCtx()
		defer cancel()

		name := fmt.Sprintf(`streamclient.test.%d.%d`, os.Getpid(), counter)
		counter++

		dataChan := acceptOne(func() (net.Listener, error) {
			return winio.ListenPipe(streamproto.LocalNamedPipePath(name), nil)
		})
		client, err := New("net.pipe:"+name, "")
		assert.Loosely(t, err, should.BeNil)

		runWireProtocolTest(ctx, dataChan, client, true)
	})
}
