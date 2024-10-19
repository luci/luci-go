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
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime/debug"

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

// acceptOne runs a trivial socket server to listen for a single connection,
// then push it over a channel.
func acceptOne(mkListen func() (net.Listener, error)) <-chan net.Conn {
	ret := make(chan net.Conn)

	listener, err := mkListen()
	if err != nil {
		panic(errors.Annotate(err, "opening listen").Err())
	}

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		if err := listener.Close(); err != nil {
			panic(errors.Annotate(err, "closing listener").Err())
		}
		go func() {
			defer close(ret)
			ret <- conn
		}()
	}()

	return ret
}

func mkTestCtx() (context.Context, func()) {
	ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
	return context.WithCancel(ctx)
}

const printQuitEnv = "WIRE_PROTOCOL_PRINT_AND_QUIT"

func init() {
	data := environ.System().Get(printQuitEnv)
	if data != "" {
		fmt.Print(data)
		os.Exit(0)
	}

	// When the test crashes (e.g. due to timebomb), dump all goroutines.
	debug.SetTraceback("all")
}

// runWireProtocolTest is used by all 'socket-like' implementations to test their
// workings.
//
// Because of the descrepancies between *nix and windows regarding the
// ForProcess Option, testing that aspect is optional.
func runWireProtocolTest(ctx context.Context, t *ftt.Test, dataChan <-chan net.Conn, client *Client, forProcessTest bool) {
	t.Run(`in-process`, func(t *ftt.Test) {
		t.Run(`text+binary`, func(t *ftt.Test) {
			go func() {
				stream, err := client.NewStream(ctx, "test")
				assert.That(t, err, should.ErrLike(nil))
				_, err = stream.Write([]byte("hello world!!"))
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, stream.Close(), should.ErrLike(nil))
			}()

			var flags streamproto.Flags
			conn := <-dataChan
			defer conn.Close()

			assert.That(t, flags.FromHandshake(conn), should.ErrLike(nil))
			assert.That(t, flags, should.Match(streamproto.Flags{
				Name:        "test",
				ContentType: "text/plain; charset=utf-8",
				Type:        streamproto.StreamType(logpb.StreamType_TEXT),
				Timestamp:   clockflag.Time(testclock.TestRecentTimeUTC),
			}))

			body, err := io.ReadAll(conn)
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, body, should.Match([]byte("hello world!!")))
		})

		t.Run(`datagram`, func(t *ftt.Test) {
			go func() {
				stream, err := client.NewDatagramStream(ctx, "test")
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, stream.WriteDatagram([]byte("hello world!!")), should.ErrLike(nil))
				assert.That(t, stream.WriteDatagram([]byte("how's the weather?")), should.ErrLike(nil))
				assert.That(t, stream.Close(), should.ErrLike(nil))
			}()

			var flags streamproto.Flags
			conn := <-dataChan
			defer conn.Close()

			assert.That(t, flags.FromHandshake(conn), should.ErrLike(nil))
			assert.That(t, flags, should.Match(streamproto.Flags{
				Name:        "test",
				ContentType: "application/x-logdog-datagram",
				Type:        streamproto.StreamType(logpb.StreamType_DATAGRAM),
				Timestamp:   clockflag.Time(testclock.TestRecentTimeUTC),
			}))

			reader := recordio.NewReader(conn, 1024)
			frame, err := reader.ReadFrameAll()
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, frame, should.Match([]byte("hello world!!")))

			frame, err = reader.ReadFrameAll()
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, frame, should.Match([]byte("how's the weather?")))
		})

		if forProcessTest {
			t.Run(`subprocess`, func(t *ftt.Test) {
				go func() {
					stream, err := client.NewStream(ctx, "test", ForProcess())
					assert.That(t, err, should.ErrLike(nil))
					assert.Loosely(t, stream, should.HaveType[*os.File])
					defer stream.Close()

					cmd := exec.Command(os.Args[0])
					cmd.Env = append(os.Environ(), printQuitEnv+"=hello world!!")
					cmd.Stdout = stream
					assert.That(t, cmd.Run(), should.ErrLike(nil))
				}()

				var flags streamproto.Flags
				conn := <-dataChan
				defer conn.Close()

				assert.That(t, flags.FromHandshake(conn), should.ErrLike(nil))
				assert.That(t, flags, should.Match(streamproto.Flags{
					Name:        "test",
					ContentType: "text/plain; charset=utf-8",
					Type:        streamproto.StreamType(logpb.StreamType_TEXT),
					Timestamp:   clockflag.Time(testclock.TestRecentTimeUTC),
				}))

				body, err := io.ReadAll(conn)
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, body, should.Match([]byte("hello world!!")))
			})

			t.Run(`in-process use of "ForProcess" handle`, func(t *ftt.Test) {
				go func() {
					stream, err := client.NewStream(ctx, "test", ForProcess())
					assert.That(t, err, should.ErrLike(nil))
					_, err = stream.Write([]byte("hello world!!"))
					assert.That(t, err, should.ErrLike(nil))
					assert.That(t, stream.Close(), should.ErrLike(nil))
				}()

				var flags streamproto.Flags
				conn := <-dataChan
				defer conn.Close()

				assert.That(t, flags.FromHandshake(conn), should.ErrLike(nil))
				assert.That(t, flags, should.Match(streamproto.Flags{
					Name:        "test",
					ContentType: "text/plain; charset=utf-8",
					Type:        streamproto.StreamType(logpb.StreamType_TEXT),
					Timestamp:   clockflag.Time(testclock.TestRecentTimeUTC),
				}))

				body, err := io.ReadAll(conn)
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, body, should.Match([]byte("hello world!!")))
			})
		}
	})
}
