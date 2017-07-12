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

	Convey(`A named pipe server`, t, func() {
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
