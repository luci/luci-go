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
	"io"
	"testing"

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

func TestFakeProtocol(t *testing.T) {
	t.Parallel()

	ftt.Run(`"fake" protocol Client`, t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestTimeUTC)

		t.Run(`good`, func(t *ftt.Test) {
			scFake, client := NewUnregisteredFake("namespace")

			t.Run(`can use a text stream`, func(t *ftt.Test) {
				stream, err := client.NewStream(ctx, "test")
				assert.Loosely(t, err, should.BeNil)

				n, err := stream.Write([]byte("hi"))
				assert.Loosely(t, n, should.Equal(2))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, stream.Close(), should.BeNil)

				streamData := scFake.Data()["namespace/test"]
				assert.Loosely(t, streamData, should.NotBeNil)
				assert.Loosely(t, streamData.GetStreamData(), should.Equal("hi"))
				assert.Loosely(t, streamData.GetDatagrams(), should.Resemble([]string{}))
				assert.Loosely(t, streamData.GetFlags(), should.Resemble(streamproto.Flags{
					Name:        "namespace/test",
					ContentType: "text/plain; charset=utf-8",
					Type:        streamproto.StreamType(logpb.StreamType_TEXT),
					Timestamp:   clockflag.Time(testclock.TestTimeUTC),
					Tags:        nil,
				}))
			})

			t.Run(`can use a binary stream`, func(t *ftt.Test) {
				stream, err := client.NewStream(ctx, "test", Binary())
				assert.Loosely(t, err, should.BeNil)

				n, err := stream.Write([]byte{0, 1, 2, 3})
				assert.Loosely(t, n, should.Equal(4))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, stream.Close(), should.BeNil)

				streamData := scFake.Data()["namespace/test"]
				assert.Loosely(t, streamData, should.NotBeNil)
				assert.Loosely(t, streamData.GetStreamData(), should.Equal("\x00\x01\x02\x03"))
				assert.Loosely(t, streamData.GetDatagrams(), should.Resemble([]string{}))
				assert.Loosely(t, streamData.GetFlags(), should.Resemble(streamproto.Flags{
					Name:        "namespace/test",
					ContentType: "application/octet-stream",
					Type:        streamproto.StreamType(logpb.StreamType_BINARY),
					Timestamp:   clockflag.Time(testclock.TestTimeUTC),
					Tags:        nil,
				}))
			})

			t.Run(`can use a datagram stream`, func(t *ftt.Test) {
				stream, err := client.NewDatagramStream(ctx, "test")
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, stream.WriteDatagram([]byte("hi")), should.BeNil)
				assert.Loosely(t, stream.WriteDatagram([]byte("there")), should.BeNil)
				assert.Loosely(t, stream.Close(), should.BeNil)

				streamData := scFake.Data()["namespace/test"]
				assert.Loosely(t, streamData, should.NotBeNil)
				assert.Loosely(t, streamData.GetStreamData(), should.BeEmpty)
				assert.Loosely(t, streamData.GetDatagrams(), should.Resemble([]string{"hi", "there"}))
				assert.Loosely(t, streamData.GetFlags(), should.Resemble(streamproto.Flags{
					Name:        "namespace/test",
					ContentType: "application/x-logdog-datagram",
					Type:        streamproto.StreamType(logpb.StreamType_DATAGRAM),
					Timestamp:   clockflag.Time(testclock.TestTimeUTC),
					Tags:        nil,
				}))
			})
		})

		t.Run(`bad`, func(t *ftt.Test) {
			t.Run(`duplicate stream`, func(t *ftt.Test) {
				_, client := NewUnregisteredFake("")

				stream, err := client.NewStream(ctx, "test")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, stream.Close(), should.BeNil)

				_, err = client.NewStream(ctx, "test")
				assert.Loosely(t, err, should.ErrLike(`stream "test": stream "test" already dialed`))

				_, err = client.NewStream(ctx, "test", Binary())
				assert.Loosely(t, err, should.ErrLike(`stream "test": stream "test" already dialed`))

				_, err = client.NewDatagramStream(ctx, "test")
				assert.Loosely(t, err, should.ErrLike(`datagram stream "test": stream "test" already dialed`))
			})

			t.Run(`simulated stream errors`, func(t *ftt.Test) {
				t.Run(`connection error`, func(t *ftt.Test) {
					scFake, client := NewUnregisteredFake("")
					scFake.SetError(errors.New("bad juju"))

					_, err := client.NewStream(ctx, "test")
					assert.Loosely(t, err, should.ErrLike(`stream "test": bad juju`))
				})

				t.Run(`use of a stream after close`, func(t *ftt.Test) {
					_, client := NewUnregisteredFake("")

					stream, err := client.NewStream(ctx, "test")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, stream.Close(), should.BeNil)

					assert.Loosely(t, stream.Close(), should.ErrLike(io.ErrClosedPipe))
					_, err = stream.Write([]byte("hi"))
					assert.Loosely(t, err, should.ErrLike(io.ErrClosedPipe))

					stream2, err := client.NewDatagramStream(ctx, "test2")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, stream2.Close(), should.BeNil)

					assert.Loosely(t, stream2.Close(), should.ErrLike(io.ErrClosedPipe))
					assert.Loosely(t, stream2.WriteDatagram([]byte("hi")), should.ErrLike(io.ErrClosedPipe))
				})
			})
		})
	})
}
