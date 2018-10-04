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

// +build darwin dragonfly freebsd linux netbsd openbsd

package streamserver

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func withTempDir(t *testing.T, fn func(string)) func() {
	return func() {
		tdir, err := ioutil.TempDir("", "butler_test")
		if err != nil {
			t.Fatalf("failed to create temporary directory: %s", err)
		}
		defer func() {
			if err := os.RemoveAll(tdir); err != nil {
				t.Errorf("failed to clean up temporary directory [%s]: %s", tdir, err)
			}
		}()
		fn(tdir)
	}
}

func TestUNIXDomainSocketServer(t *testing.T) {
	t.Parallel()

	Convey(`A UNIX domain socket server`, t, func() {
		ctx := context.Background()

		Convey(`Will refuse to create if there is an empty path.`, func() {
			_, err := NewUNIXDomainSocketServer(ctx, "")
			So(err, ShouldErrLike, "cannot have empty path")
		})

		Convey(`Will refuse to create if longer than maximum length.`, func() {
			_, err := NewUNIXDomainSocketServer(ctx, strings.Repeat("A", maxPOSIXNamedSocketLength+1))
			So(err, ShouldErrLike, "path exceeds maximum length")
		})

		Convey(`When created and listening.`, withTempDir(t, func(tdir string) {
			svr, err := NewUNIXDomainSocketServer(ctx, filepath.Join(tdir, "butler.sock"))
			So(err, ShouldBeNil)

			So(svr.Listen(), ShouldBeNil)
			defer svr.Close()

			client, err := streamclient.New(svr.Address())
			So(err, ShouldBeNil)

			testClientServer(t, svr, client)
		}))
	})
}
