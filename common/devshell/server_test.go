// Copyright 2017 The LUCI Authors.
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

package devshell

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"testing"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

)

func TestServerLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Serve or close before init", t, func() {
		s := Server{}
		So(s.Serve(), ShouldErrLike, "not initialized")
		So(s.Close(), ShouldErrLike, "not initialized")
	})

	Convey("Double init", t, func() {
		s := Server{}
		defer s.Close()
		_, err := s.Initialize(ctx)
		So(err, ShouldBeNil)
		_, err = s.Initialize(ctx)
		So(err, ShouldErrLike, "already initialized")
	})

	Convey("Server after close", t, func() {
		s := Server{}
		_, err := s.Initialize(ctx)
		So(err, ShouldBeNil)
		So(s.Close(), ShouldBeNil)
		So(s.Serve(), ShouldErrLike, "already closed")
	})
}

func TestProtocol(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

	Convey("With server", t, func(c C) {
		s := Server{
			Source: tokenSource{
				token: &oauth2.Token{
					AccessToken: "tok1",
					Expiry:      clock.Now(ctx).Add(30 * time.Minute),
				},
			},
			Email: "some@example.com",
		}
		p, err := s.Initialize(ctx)
		So(err, ShouldBeNil)

		done := make(chan struct{})
		go func() {
			s.Serve()
			close(done)
		}()
		defer func() {
			s.Close()
			<-done
		}()

		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", p.Port))
		if err != nil {
			panic(err)
		}

		Convey("Happy path", func() {
			So(call(conn, "[]"), ShouldEqual, `["some@example.com",null,"tok1",1800]`)
		})

		Convey("Wrong format", func() {
			So(call(conn, "{BADJSON"), ShouldEqual, ``)
		})
	})
}

type tokenSource struct {
	token *oauth2.Token
}

func (t tokenSource) Token() (*oauth2.Token, error) {
	return t.token, nil
}

func call(conn net.Conn, req string) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%d\n", len(req)))
	buf.Write([]byte(req))
	if _, err := conn.Write(buf.Bytes()); err != nil {
		panic(err)
	}

	blob, err := ioutil.ReadAll(conn)
	if err != nil {
		panic(err)
	}

	str := strings.SplitN(string(blob), "\n", 2)
	if len(str) != 2 {
		panic(err)
	}

	_, err = strconv.Atoi(str[0])
	if err != nil {
		panic(err)
	}

	return str[1]
}
