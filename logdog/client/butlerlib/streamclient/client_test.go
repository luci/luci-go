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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestClientGeneral(t *testing.T) {
	t.Parallel()

	ftt.Run(`General Client checks`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`fails to instantiate a Client with an invalid protocol.`, func(t *ftt.Test) {
			_, err := New("notreal:foo", "")
			assert.Loosely(t, err, should.ErrLike("no protocol registered for [notreal]"))
		})

		scFake := NewFake()
		defer scFake.Unregister()

		t.Run(`ForProcess used with datagram stream`, func(t *ftt.Test) {
			client, err := New(scFake.StreamServerPath(), "")
			assert.Loosely(t, err, should.BeNil)

			_, err = client.NewDatagramStream(ctx, "test", ForProcess())
			assert.Loosely(t, err, should.ErrLike("cannot specify ForProcess on a datagram stream"))
		})

		t.Run(`bad options`, func(t *ftt.Test) {
			client, err := New(scFake.StreamServerPath(), "")
			assert.Loosely(t, err, should.BeNil)

			_, err = client.NewStream(ctx, "test", WithTags("bad+@!tag", "value"))
			assert.Loosely(t, err, should.ErrLike(`invalid tag "bad+@!tag"`))

			// for coverage, whee.
			_, err = client.NewStream(ctx, "test", WithTags("bad+@!tag", "value"), Binary())
			assert.Loosely(t, err, should.ErrLike(`invalid tag "bad+@!tag"`))

			_, err = client.NewDatagramStream(ctx, "test", WithTags("bad+@!tag", "value"))
			assert.Loosely(t, err, should.ErrLike(`invalid tag "bad+@!tag"`))
		})

		t.Run(`simulated stream errors`, func(t *ftt.Test) {
			t.Run(`connection error`, func(t *ftt.Test) {
				client, err := New(scFake.StreamServerPath(), "")
				assert.Loosely(t, err, should.BeNil)
				scFake.SetError(errors.New("bad juju"))

				_, err = client.NewStream(ctx, "test")
				assert.Loosely(t, err, should.ErrLike(`stream "test": bad juju`))
			})
		})
	})
}
