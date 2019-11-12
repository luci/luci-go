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

	"go.chromium.org/luci/common/errors"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func makeServer(ctx context.Context, c C) *Server {
	server, err := NewServer(ctx, ServerConfig{AuthToken: "hello"})
	c.So(err, ShouldBeNil)
	return server
}

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
		Convey("Call Start twice", func(c C) {
			ctx := context.Background()
			server := makeServer(ctx, c)
			defer server.Close()

			err := server.Start(ctx)
			So(err, ShouldBeNil)
			err = server.Start(ctx)
			So(err, ShouldErrLike, "cannot call Start twice")
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Close check", t, func(c C) {
		ctx := context.Background()
		server := makeServer(ctx, c)
		defer server.Close()

		err := server.Start(ctx)
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
	ctx := context.Background()
	Convey("Run check", t, func() {
		Convey("Server error", func(c C) {
			server := makeServer(ctx, c)
			defer server.Close()

			fakeError := errors.New("test error")
			server.errC <- fakeError

			err := server.Run(ctx, func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			})
			So(err, ShouldErrLike, fakeError)
		})

		Convey("Callback finished", func(c C) {
			server := makeServer(ctx, c)
			defer server.Close()

			err := server.Run(ctx, func(ctx context.Context) error {
				return nil
			})
			So(err, ShouldBeNil)
		})
	})
}

func TestMessageProcessing(t *testing.T) {
	ctx := context.Background()
	badInput := `blah`
	goodInput := `{"testResult":{"testPath":"foo/bar/baz","resultId":"result000001","expected":true,"status":"PASS","summaryMarkdown":"hello","startTime":"2019-11-12T00:02:54.855213790Z","tags":[{"key":"foo","value":"bar"}]}}{"testResult":{"testPath":"sdhg/jgdsh/yeuwt","resultId":"result000002","status":"FAIL","summaryMarkdown":"iuuujn","startTime":"2019-11-12T00:02:54.855214521Z","tags":[{"key":"dskhnfjsd","value":"bar"}]}}`

	Convey("Message-processing check", t, func() {
		Convey("Garbage data", func() {
			dc := json.NewDecoder(strings.NewReader(badInput))
			err := processMessages(ctx, dc)
			So(err, ShouldErrLike, "invalid")
		})

		Convey("Two populated messages", func() {
			dc := json.NewDecoder(strings.NewReader(goodInput))
			err := processMessages(ctx, dc)
			So(err, ShouldErrLike, io.EOF)
		})

		Convey("Context cancellation", func() {
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			dc := json.NewDecoder(strings.NewReader(goodInput))
			err := processMessages(ctx, dc)
			So(err, ShouldBeNil)
		})
	})
}

func TestConnection(t *testing.T) {
	ctx := context.Background()
	Convey("Connection-handling check", t, func() {
		Convey("Bad handshake", func(c C) {
			server := makeServer(ctx, c)
			err := server.Start(ctx)
			defer server.Close()
			So(err, ShouldBeNil)

			addr := fmt.Sprintf(":%d", server.Config().Port)
			conn, err := net.Dial("tcp", addr)
			So(err, ShouldBeNil)

			_, err = conn.Write([]byte(`{"auth_token":"BAD"}`))
			So(err, ShouldBeNil)
			So(<-server.ErrC(), ShouldErrLike, "handshake failed")

		})

		Convey("Bad test result", func(c C) {
			server := makeServer(ctx, c)
			err := server.Start(ctx)
			defer server.Close()
			So(err, ShouldBeNil)

			addr := fmt.Sprintf(":%d", server.Config().Port)
			conn, err := net.Dial("tcp", addr)
			So(err, ShouldBeNil)

			_, err = conn.Write([]byte(`{"auth_token":"hello"}`))
			So(err, ShouldBeNil)

			_, err = conn.Write([]byte("kjhdsghg"))
			So(err, ShouldBeNil)
			So(<-server.ErrC(), ShouldErrLike, "invalid")
		})
	})
}
