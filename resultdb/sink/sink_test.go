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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
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

func TestClose(t *testing.T) {
	Convey("Close check", t, func() {
		ctx := context.Background()
		ctx = gologger.StdConfig.Use(ctx)
		ctx = logging.SetLevel(ctx, logging.Debug)
		server, err := NewServer(ctx, ServerConfig{AuthToken: "hello"})
		So(err, ShouldBeNil)
		server.Serve(ctx)
		addr := fmt.Sprintf(":%d", server.Config().Port)
		conn, err := net.Dial("tcp", addr)
		So(err, ShouldBeNil)
		_, err = conn.Write([]byte(`{"auth_token":"hello"}`))
		So(err, ShouldBeNil)
		err = server.Close(ctx)
		So(err, ShouldBeNil)
		_, err = net.Dial("tcp", addr)
		So(err, ShouldErrLike, "connection refused")
	})
}
