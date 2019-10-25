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
	"net"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
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
	Convey("Successful handshake", t, func() {
		ctx := context.Background()
		ctx = memlogger.Use(ctx)
		var s Server
		s.cfg.AuthToken = "hello"
		sc, cc := net.Pipe()
		done := make(chan bool)
		go func() {
			s.handleConnection(ctx, sc)
			done <- true
		}()
		msg := []byte("{\"auth_token\":\"hello\"}")
		_, err := cc.Write(msg)
		So(err, ShouldBeNil)
		<-done
		log := logging.Get(ctx).(*memlogger.MemLogger)
		So(log, memlogger.ShouldNotHaveLog, logging.Error)
	})

	Convey("Failed handshake", t, func() {
		ctx := context.Background()
		ctx = memlogger.Use(ctx)
		var s Server
		s.cfg.AuthToken = "hello"
		sc, cc := net.Pipe()
		done := make(chan bool)
		go func() {
			s.handleConnection(ctx, sc)
			done <- true
		}()
		msg := []byte("{\"auth_token\":\"BAD\"}")
		_, err := cc.Write(msg)
		So(err, ShouldBeNil)
		<-done
		log := logging.Get(ctx).(*memlogger.MemLogger)
		So(log, memlogger.ShouldHaveLog, logging.Error)
	})
}
