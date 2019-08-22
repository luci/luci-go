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

package streamclient

import (
	"net"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTcp4Client(t *testing.T) {
	t.Parallel()

	Convey(`test tcp4`, t, func() {
		ctx, cancel := mkTestCtx()
		defer cancel()

		dataChan, addr, closer := acceptOn(ctx, func() (net.Listener, error) {
			var lc net.ListenConfig
			return lc.Listen(ctx, "tcp4", "localhost:")
		})
		defer closer()
		client, err := New("tcp4:"+addr.String(), "")
		So(err, ShouldBeNil)

		runWireProtocolTest(ctx, dataChan, client, runtime.GOOS != "windows")
	})
}

func TestTcp6Client(t *testing.T) {
	t.Parallel()

	Convey(`test tcp6`, t, func() {
		ctx, cancel := mkTestCtx()
		defer cancel()

		dataChan, addr, closer := acceptOn(ctx, func() (net.Listener, error) {
			var lc net.ListenConfig
			return lc.Listen(ctx, "tcp6", "[::1]:")
		})
		defer closer()
		client, err := New("tcp6:"+addr.String(), "")
		So(err, ShouldBeNil)

		runWireProtocolTest(ctx, dataChan, client, runtime.GOOS != "windows")
	})
}
