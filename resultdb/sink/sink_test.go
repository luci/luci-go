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
	"net"
	"strings"
	"testing"

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
