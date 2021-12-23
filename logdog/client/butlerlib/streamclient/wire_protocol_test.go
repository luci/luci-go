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
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"runtime/debug"

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"

	. "github.com/smartystreets/goconvey/convey"
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
	data := environ.System().GetEmpty(printQuitEnv)
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
func runWireProtocolTest(ctx context.Context, dataChan <-chan net.Conn, client *Client, forProcessTest bool) {
	Convey(`in-process`, func() {
		Convey(`text+binary`, func(c C) {
			go func() {
				stream, err := client.NewStream(ctx, "test")
				c.So(err, ShouldBeNil)
				_, err = stream.Write([]byte("hello world!!"))
				c.So(err, ShouldBeNil)
				c.So(stream.Close(), ShouldBeNil)
			}()

			var flags streamproto.Flags
			conn := <-dataChan
			defer conn.Close()

			So(flags.FromHandshake(conn), ShouldBeNil)
			So(flags, ShouldResemble, streamproto.Flags{
				Name:        "test",
				ContentType: "text/plain; charset=utf-8",
				Type:        streamproto.StreamType(logpb.StreamType_TEXT),
				Timestamp:   clockflag.Time(testclock.TestRecentTimeUTC),
			})

			body, err := ioutil.ReadAll(conn)
			So(err, ShouldBeNil)
			So(body, ShouldResemble, []byte("hello world!!"))
		})

		Convey(`datagram`, func(c C) {
			go func() {
				stream, err := client.NewDatagramStream(ctx, "test")
				c.So(err, ShouldBeNil)
				c.So(stream.WriteDatagram([]byte("hello world!!")), ShouldBeNil)
				c.So(stream.WriteDatagram([]byte("how's the weather?")), ShouldBeNil)
				c.So(stream.Close(), ShouldBeNil)
			}()

			var flags streamproto.Flags
			conn := <-dataChan
			defer conn.Close()

			So(flags.FromHandshake(conn), ShouldBeNil)
			So(flags, ShouldResemble, streamproto.Flags{
				Name:        "test",
				ContentType: "application/x-logdog-datagram",
				Type:        streamproto.StreamType(logpb.StreamType_DATAGRAM),
				Timestamp:   clockflag.Time(testclock.TestRecentTimeUTC),
			})

			reader := recordio.NewReader(conn, 1024)
			frame, err := reader.ReadFrameAll()
			So(err, ShouldBeNil)
			So(frame, ShouldResemble, []byte("hello world!!"))

			frame, err = reader.ReadFrameAll()
			So(err, ShouldBeNil)
			So(frame, ShouldResemble, []byte("how's the weather?"))
		})

		if forProcessTest {
			Convey(`subprocess`, func(c C) {
				go func() {
					stream, err := client.NewStream(ctx, "test", ForProcess())
					c.So(err, ShouldBeNil)
					c.So(stream, ShouldHaveSameTypeAs, (*os.File)(nil))
					defer stream.Close()

					cmd := exec.Command(os.Args[0])
					cmd.Env = append(os.Environ(), printQuitEnv+"=hello world!!")
					cmd.Stdout = stream
					c.So(cmd.Run(), ShouldBeNil)
				}()

				var flags streamproto.Flags
				conn := <-dataChan
				defer conn.Close()

				So(flags.FromHandshake(conn), ShouldBeNil)
				So(flags, ShouldResemble, streamproto.Flags{
					Name:        "test",
					ContentType: "text/plain; charset=utf-8",
					Type:        streamproto.StreamType(logpb.StreamType_TEXT),
					Timestamp:   clockflag.Time(testclock.TestRecentTimeUTC),
				})

				body, err := ioutil.ReadAll(conn)
				So(err, ShouldBeNil)
				So(body, ShouldResemble, []byte("hello world!!"))
			})

			Convey(`in-process use of "ForProcess" handle`, func(c C) {
				go func() {
					stream, err := client.NewStream(ctx, "test", ForProcess())
					c.So(err, ShouldBeNil)
					_, err = stream.Write([]byte("hello world!!"))
					c.So(err, ShouldBeNil)
					c.So(stream.Close(), ShouldBeNil)
				}()

				var flags streamproto.Flags
				conn := <-dataChan
				defer conn.Close()

				So(flags.FromHandshake(conn), ShouldBeNil)
				So(flags, ShouldResemble, streamproto.Flags{
					Name:        "test",
					ContentType: "text/plain; charset=utf-8",
					Type:        streamproto.StreamType(logpb.StreamType_TEXT),
					Timestamp:   clockflag.Time(testclock.TestRecentTimeUTC),
				})

				body, err := ioutil.ReadAll(conn)
				So(err, ShouldBeNil)
				So(body, ShouldResemble, []byte("hello world!!"))
			})
		}
	})
}
