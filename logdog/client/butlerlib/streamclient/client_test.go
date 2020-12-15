// Copyright 2015 The LUCI Authors.
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
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestClientGeneral(t *testing.T) {
	t.Parallel()

	Convey(`General Client checks`, t, func() {
		ctx := context.Background()

		Convey(`fails to instantiate a Client with an invalid protocol.`, func() {
			_, err := New("notreal:foo", "")
			So(err, ShouldErrLike, "no protocol registered for [notreal]")
		})

		Convey(`ForProcess used with datagram stream`, func() {
			client := NewFake("")

			_, err := client.NewDatagramStream(ctx, "test", ForProcess())
			So(err, ShouldErrLike, "cannot specify ForProcess on a datagram stream")
		})

		Convey(`bad options`, func() {
			client := NewFake("")

			_, err := client.NewStream(ctx, "test", WithTags("bad+@!tag", "value"))
			So(err, ShouldErrLike, `invalid tag "bad+@!tag"`)

			// for coverage, whee.
			_, err = client.NewStream(ctx, "test", WithTags("bad+@!tag", "value"), Binary())
			So(err, ShouldErrLike, `invalid tag "bad+@!tag"`)

			_, err = client.NewDatagramStream(ctx, "test", WithTags("bad+@!tag", "value"))
			So(err, ShouldErrLike, `invalid tag "bad+@!tag"`)
		})

		Convey(`simulated stream errors`, func() {
			Convey(`connection error`, func() {
				client := NewFake("")
				client.SetFakeError(errors.New("bad juju"))

				_, err := client.NewStream(ctx, "test")
				So(err, ShouldErrLike, `stream "test": bad juju`)
			})
		})
	})
}
