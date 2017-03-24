// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package streamserver

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/luci/luci-go/logdog/client/butlerlib/streamclient"

	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestWindowsNamedPipeServer(t *testing.T) {
	t.Parallel()

	pid := os.Getpid()

	// TODO(dnj): Re-enable after switching back to "winio" pending bug.
	// See: crbug.com/702105
	SkipConvey(`A named pipe server`, t, func() {
		ctx := context.Background()

		Convey(`Will refuse to create if there is an empty path.`, func() {
			_, err := NewNamedPipeServer(ctx, "")
			So(err, ShouldErrLike, "cannot have empty name")
		})

		Convey(`Will refuse to create if longer than maximum length.`, func() {
			_, err := NewNamedPipeServer(ctx, strings.Repeat("A", maxWindowsNamedPipeLength+1))
			So(err, ShouldErrLike, "name exceeds maximum length")
		})

		Convey(`When created and listening.`, func() {
			svr, err := NewNamedPipeServer(ctx, fmt.Sprintf("ButlerNamedPipeTest_%d", pid))
			So(err, ShouldBeNil)

			So(svr.Listen(), ShouldBeNil)
			defer svr.Close()

			client, err := streamclient.New(svr.Address())
			So(err, ShouldBeNil)

			testClientServer(t, svr, client)
		})
	})
}
