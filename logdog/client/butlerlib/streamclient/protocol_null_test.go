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
	"context"
	"os"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNullProtocol(t *testing.T) {
	ftt.Run(`"null" protocol Client`, t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestTimeUTC)

		t.Run(`good`, func(t *ftt.Test) {
			client, err := New("null", "namespace")
			assert.Loosely(t, err, should.BeNil)

			t.Run(`can use a text stream`, func(t *ftt.Test) {
				stream, err := client.NewStream(ctx, "test")
				assert.Loosely(t, err, should.BeNil)

				n, err := stream.Write([]byte("hi"))
				assert.Loosely(t, n, should.Equal(2))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, stream.Close(), should.BeNil)
			})

			t.Run(`can use a text stream for a subprocess`, func(t *ftt.Test) {
				stream, err := client.NewStream(ctx, "test", ForProcess())
				assert.Loosely(t, err, should.BeNil)
				defer stream.Close()
				assert.Loosely(t, stream, should.HaveType[*os.File])

				n, err := stream.Write([]byte("hi"))
				assert.Loosely(t, n, should.Equal(2))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, stream.Close(), should.BeNil)
				assert.Loosely(t, stream.Close(), should.ErrLike(os.ErrClosed))
			})

			t.Run(`can use a binary stream`, func(t *ftt.Test) {
				stream, err := client.NewStream(ctx, "test", Binary())
				assert.Loosely(t, err, should.BeNil)

				n, err := stream.Write([]byte{0, 1, 2, 3})
				assert.Loosely(t, n, should.Equal(4))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, stream.Close(), should.BeNil)
			})

			t.Run(`can use a datagram stream`, func(t *ftt.Test) {
				stream, err := client.NewDatagramStream(ctx, "test")
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, stream.WriteDatagram([]byte("hi")), should.BeNil)
				assert.Loosely(t, stream.WriteDatagram([]byte("there")), should.BeNil)
				assert.Loosely(t, stream.Close(), should.BeNil)
			})
		})
	})
}
