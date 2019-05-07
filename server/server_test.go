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

package server

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"testing"

	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
)

func TestServer(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		srv := New(Options{
			HTTPAddr:  "main_addr",
			AdminAddr: "admin_addr",

			// Bind to auto-assigned ports.
			testListeners: map[string]net.Listener{
				"main_addr":  setupListener(),
				"admin_addr": setupListener(),
			},
		})

		mainPort := srv.opts.testListeners["main_addr"].Addr().(*net.TCPAddr).Port
		mainAddr := fmt.Sprintf("http://127.0.0.1:%d", mainPort)

		srv.Routes.GET("/test", router.MiddlewareChain{}, func(c *router.Context) {
			c.Writer.Write([]byte("Hello, world"))
		})

		// Run the serving loop in parallel.
		done := make(chan error)
		go func() { done <- srv.ListenAndServe() }()

		// Call "/test" to verify the server can serve successfully.
		resp, err := httpGet(mainAddr + "/test")
		So(err, ShouldBeNil)
		So(resp, ShouldEqual, "Hello, world")

		// Make sure Shutdown works.
		srv.Shutdown()
		So(<-done, ShouldBeNil)
	})
}

func setupListener() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	return l
}

func httpGet(addr string) (resp string, err error) {
	res, err := http.Get(addr)
	if err != nil {
		return
	}
	defer res.Body.Close()
	blob, err := ioutil.ReadAll(res.Body)
	return string(blob), err
}
