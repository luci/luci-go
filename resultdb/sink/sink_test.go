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

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/errors"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDefaultServerConfig(t *testing.T) {
	Convey("NewServer check", t, func() {
		s, err := NewServer(context.Background(), ServerConfig{})
		So(err, ShouldBeNil)

		cfg := s.Config()
		So(cfg.AuthToken, ShouldNotBeEmpty)
	})
}

func TestHandshake(t *testing.T) {
	doCheck := func(msg, token string) error {
		dc := json.NewDecoder(strings.NewReader(msg))
		return processHandshake(dc, token)
	}

	Convey("Handshake check", t, func() {
		authToken := "hello"
		Convey("Successful handshake", func() {
			err := doCheck(`{"auth_token":"hello"}`, authToken)
			So(err, ShouldBeNil)
		})
		Convey("Unsuccessful handshake", func() {
			err := doCheck(`{"auth_token":"BAD"}`, authToken)
			So(err, ShouldErrLike, "invalid AuthToken")
			err = doCheck(`garbage`, authToken)
			So(err, ShouldErrLike, "failed to parse")
		})
	})
}

func TestStart(t *testing.T) {
	Convey("Start check", t, func() {
		Convey("Call Start twice", func() {
			ctx := context.Background()
			server, err := NewServer(ctx, ServerConfig{AuthToken: "hello"})
			So(err, ShouldBeNil)
			defer server.Close()

			err = server.Start(ctx)
			So(err, ShouldBeNil)
			err = server.Start(ctx)
			So(err, ShouldErrLike, "cannot call Start twice")
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Close check", t, func() {
		ctx := context.Background()
		server, err := NewServer(ctx, ServerConfig{AuthToken: "hello"})
		So(err, ShouldBeNil)
		defer server.Close()

		err = server.Start(ctx)
		So(err, ShouldBeNil)

		addr := fmt.Sprintf(":%d", server.Config().Port)
		conn, err := net.Dial("tcp", addr)
		So(err, ShouldBeNil)

		_, err = conn.Write([]byte(`{"auth_token":"hello"}`))
		So(err, ShouldBeNil)

		err = server.Close()
		So(err, ShouldBeNil)

		_, err = net.Dial("tcp", addr)
		So(err, ShouldErrLike, "connection refused")
	})
}

func TestRun(t *testing.T) {
	Convey("Run check", t, func() {
		Convey("Server error", func() {
			ctx := context.Background()
			server, err := NewServer(ctx, ServerConfig{AuthToken: "hello"})
			So(err, ShouldBeNil)
			defer server.Close()

			fakeError := errors.New("test error")
			server.errC <- fakeError

			err = server.Run(ctx, func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			})
			So(err, ShouldErrLike, fakeError)
		})

		Convey("Callback finished", func() {
			ctx := context.Background()
			server, err := NewServer(ctx, ServerConfig{AuthToken: "hello"})
			So(err, ShouldBeNil)
			defer server.Close()

			err = server.Run(ctx, func(ctx context.Context) error {
				return nil
			})
			So(err, ShouldBeNil)
		})
	})
}
