// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd

package streamserver

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luci/luci-go/logdog/client/butlerlib/streamclient"

	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
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
