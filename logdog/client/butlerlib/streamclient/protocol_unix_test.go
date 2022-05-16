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

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd
// +build darwin dragonfly freebsd linux netbsd openbsd

package streamclient

import (
	"io/ioutil"
	"net"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUnixClient(t *testing.T) {
	t.Parallel()

	Convey(`test unix`, t, func() {
		defer timebomb()()

		ctx, cancel := mkTestCtx()
		defer cancel()

		f, err := ioutil.TempFile("", "")
		So(err, ShouldBeNil)
		name := f.Name()
		So(f.Close(), ShouldBeNil)
		So(os.Remove(name), ShouldBeNil)

		dataChan := acceptOne(func() (net.Listener, error) {
			var lc net.ListenConfig
			return lc.Listen(ctx, "unix", name)
		})
		client, err := New("unix:"+name, "")
		So(err, ShouldBeNil)

		runWireProtocolTest(ctx, dataChan, client, true)
	})
}
