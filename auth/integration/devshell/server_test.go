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
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestProtocol(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

	ftt.Run("With server", t, func(c *ftt.Test) {
		s := Server{
			Source: oauth2.StaticTokenSource(&oauth2.Token{
				AccessToken: "tok1",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}),
			Email: "some@example.com",
		}
		p, err := s.Start(ctx)
		assert.Loosely(c, err, should.BeNil)
		defer s.Stop(ctx)

		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", p.Port))
		if err != nil {
			panic(err)
		}
		defer conn.Close()

		c.Run("Happy path", func(c *ftt.Test) {
			assert.Loosely(c, call(conn, "[]"), should.Equal(`["some@example.com",null,"tok1",1800]`))
		})

		c.Run("Wrong format", func(c *ftt.Test) {
			assert.Loosely(c, call(conn, "{BADJSON"), should.Equal(`["failed to deserialize from JSON: invalid character 'B' looking for beginning of object key string"]`))
		})
	})
}

func call(conn net.Conn, req string) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%d\n", len(req)))
	buf.Write([]byte(req))
	if _, err := conn.Write(buf.Bytes()); err != nil {
		panic(err)
	}

	blob, err := io.ReadAll(conn)
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
