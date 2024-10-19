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

//go:build unix
// +build unix

package streamclient

import (
	"io/ioutil"
	"net"
	"os"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestUnixClient(t *testing.T) {
	t.Parallel()

	ftt.Run(`test unix`, t, func(t *ftt.Test) {
		defer timebomb()()

		ctx, cancel := mkTestCtx()
		defer cancel()

		f, err := ioutil.TempFile("", "")
		assert.Loosely(t, err, should.BeNil)
		name := f.Name()
		assert.Loosely(t, f.Close(), should.BeNil)
		assert.Loosely(t, os.Remove(name), should.BeNil)

		dataChan := acceptOne(func() (net.Listener, error) {
			var lc net.ListenConfig
			return lc.Listen(ctx, "unix", name)
		})
		client, err := New("unix:"+name, "")
		assert.Loosely(t, err, should.BeNil)

		runWireProtocolTest(ctx, t, dataChan, client, true)
	})
}
