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

//go:build unix
// +build unix

package streamserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

// shortTempDir is like t.TempDir, except that it returns a directory path short
// enough to work as the root for a unix domain socket. t.TempDir can return
// a long path which fails on macOS.
func shortTempDir(t testing.TB) string {
	t.Helper()
	tdir, err := os.MkdirTemp("", "butler_test")
	assert.That(t, err, should.ErrLike(nil), truth.LineContext())
	t.Cleanup(func() {
		err := os.RemoveAll(tdir)
		assert.That(t, err, should.ErrLike(nil), truth.LineContext())
	})
	return tdir
}

func TestUNIXDomainSocketServer(t *testing.T) {
	t.Parallel()

	ftt.Run(`A UNIX domain socket server`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`Will create a temporary name if given an empty path`, func(t *ftt.Test) {
			s, err := newStreamServer(ctx, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.Address(), should.HavePrefix("unix:"+os.TempDir()))
		})

		t.Run(`Will refuse to create if longer than maximum length.`, func(t *ftt.Test) {
			_, err := newStreamServer(ctx, strings.Repeat("A", maxPOSIXNamedSocketLength+1))
			assert.Loosely(t, err, should.ErrLike("path exceeds maximum length"))
		})

		t.Run(`When created and listening.`, func(t *ftt.Test) {
			tdir := shortTempDir(t)
			svr, err := newStreamServer(ctx, filepath.Join(tdir, "butler.sock"))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, svr.Listen(), should.BeNil)
			defer svr.Close()

			client, err := streamclient.New(svr.Address(), "")
			assert.Loosely(t, err, should.BeNil)

			testClientServer(t, svr, client)
		})
	})
}
