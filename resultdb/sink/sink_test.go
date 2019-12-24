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

package sink

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/lucictx"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func handshakeCheck(msg, token string) error {
	dc := json.NewDecoder(strings.NewReader(msg))
	return processHandshake(dc, token)
}

func Test(t *testing.T) {
	Convey("Test Server", t, func() {
		ctx := context.Background()

		// Create a 5-second timeout to try to catch hanging tests.
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
				So(ctx.Err(), ShouldNotBeNil)
			}
		}()

		Convey("Default server config", func() {
			s, err := NewServer(ctx, ServerConfig{})
			So(err, ShouldBeNil)

			cfg := s.Config()
			So(cfg.AuthToken, ShouldNotBeEmpty)
		})

		Convey("Handshake processing", func() {
			authToken := "hello"
			Convey("Successful handshake", func() {
				So(handshakeCheck(`{"auth_token":"hello"}`, authToken),
					ShouldBeNil)
			})
			Convey("Unsuccessful handshake", func() {
				err := handshakeCheck(`{"auth_token":"BAD"}`, authToken)
				So(err, ShouldErrLike, "invalid AuthToken")
				err = handshakeCheck(`garbage`, authToken)
				So(err, ShouldErrLike, "failed to parse")
			})
		})

		Convey("Tests with a real server", func() {
			server, err := NewServer(ctx, ServerConfig{AuthToken: "hello"})
			So(err, ShouldBeNil)

			Convey("Tests with Start", func() {
				So(server.Start(ctx), ShouldBeNil)
				defer server.Close()

				Convey("Calling Start twice is impossible", func() {
					So(server.Start(ctx), ShouldErrLike, "cannot call Start twice")
				})

				Convey("Close stops new connections", func() {
					addr := fmt.Sprintf(":%d", server.Config().Port)
					conn, err := net.Dial("tcp", addr)
					So(err, ShouldBeNil)

					_, err = conn.Write([]byte(`{"auth_token":"hello"}`))
					So(err, ShouldBeNil)

					err = server.Close()
					So(err, ShouldBeNil)

					_, err = net.Dial("tcp", addr)
					So(err, ShouldNotBeNil)
				})

			})

			Convey("Connection handling tests", func() {
				cr, cw := net.Pipe()
				defer cr.Close()
				defer cw.Close()
				errC := make(chan error)
				ctx, cancel := context.WithCancel(ctx)
				go func() { errC <- server.handleConnection(ctx, cr) }()

				Convey("Bad handshake", func() {
					_, err := cw.Write([]byte(`{"auth_token":"BAD"}`))
					So(err, ShouldBeNil)
					So(<-errC, ShouldErrLike, "handshake failed")
				})

				Convey("Bad test result", func() {
					_, err := cw.Write([]byte(`{"auth_token":"hello"}`))
					So(err, ShouldBeNil)

					_, err = cw.Write([]byte("kjhdsghg"))
					So(err, ShouldBeNil)
					So(<-errC, ShouldErrLike, "invalid")
				})

				Convey("Context cancellation", func() {
					// With context cancellation, the failure mode is that the server
					// ignores it and keeps running forever. To test this we cancel the
					// top-level context and wait on the error channel. If the connection
					// handler is hung, the top-level test timeout will fail.
					cancel()
					So(<-errC, ShouldNotBeNil)
				})
			})

			Convey("Tests with Run", func() {
				Convey("Run aborts after server error", func() {
					fakeError := errors.New("test error")
					server.errC <- fakeError
					err := server.Run(ctx, func(ctx context.Context) error {
						<-ctx.Done()
						return ctx.Err()
					})
					So(err, ShouldErrLike, fakeError)
				})
				Convey("Run stops after callback completes", func() {
					err := server.Run(ctx, func(ctx context.Context) error {
						return nil
					})
					So(err, ShouldBeNil)
				})
			})
		})

		Convey("Message-processing check", func() {
			badInput := `blah`
			goodInput := `{"testResult":{"testId":"foo/bar/baz","resultId":"result000001","expected":true,"status":"PASS","summaryHtml":"hello","startTime":"2019-11-12T00:02:54.855213790Z","tags":[{"key":"foo","value":"bar"}]}}{"testResult":{"testId":"sdhg/jgdsh/yeuwt","resultId":"result000002","status":"FAIL","summaryHtml":"iuuujn","startTime":"2019-11-12T00:02:54.855214521Z","tags":[{"key":"dskhnfjsd","value":"bar"}]}}`
			Convey("Garbage data", func() {
				dc := json.NewDecoder(strings.NewReader(badInput))
				err := processMessages(dc)
				So(err, ShouldErrLike, "invalid")
			})

			Convey("Two populated messages", func() {
				dc := json.NewDecoder(strings.NewReader(goodInput))
				err := processMessages(dc)
				So(err, ShouldErrLike, io.EOF)
			})
		})
	})
}

func TestExport(t *testing.T) {
	Convey("Export check", t, func() {
		ctx := context.Background()
		s, err := NewServer(ctx, ServerConfig{Port: 42, AuthToken: "hello"})
		So(err, ShouldBeNil)
		ctx = s.Export(ctx)
		db := lucictx.GetResultDB(ctx)
		So(db, ShouldNotBeNil)
		So(db.TestResults, ShouldNotBeNil)
		So(db.TestResults.Port, ShouldEqual, 42)
		So(db.TestResults.AuthToken, ShouldEqual, "hello")
	})
}
