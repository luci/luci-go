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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"

	. "github.com/smartystreets/goconvey/convey"
)

func TestServer(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		tmpSecret, err := tempSecret()
		So(err, ShouldBeNil)
		defer os.Remove(tmpSecret.Name())

		srv := New(Options{
			HTTPAddr:       "main_addr",
			AdminAddr:      "admin_addr",
			Prod:           true,
			RootSecretPath: tmpSecret.Name(),

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

		srv.Routes.GET("/secret", router.MiddlewareChain{}, func(c *router.Context) {
			s, err := secrets.GetSecret(c.Context, "secret_name")
			if err != nil {
				c.Writer.WriteHeader(500)
			} else {
				c.Writer.Write([]byte(s.Current))
			}
		})

		// Run the serving loop in parallel.
		serveErr := errorEvent{signal: make(chan struct{})}
		go func() { serveErr.Set(srv.ListenAndServe()) }()

		// Call "/test" to verify the server can serve successfully.
		resp, err := httpGet(mainAddr+"/test", &serveErr)
		So(err, ShouldBeNil)
		So(resp, ShouldEqual, "Hello, world")

		// Secrets store is working.
		resp, err = httpGet(mainAddr+"/secret", &serveErr)
		So(err, ShouldBeNil)
		So(resp, ShouldHaveLength, 16)

		// Make sure Shutdown works.
		srv.Shutdown()
		So(serveErr.Get(), ShouldBeNil)
	})
}

func tempSecret() (out *os.File, err error) {
	var f *os.File
	defer func() {
		if f != nil && err != nil {
			os.Remove(f.Name())
		}
	}()
	f, err = ioutil.TempFile("", "luci-server-test")
	if err != nil {
		return nil, err
	}
	secret := secrets.Secret{Current: []byte("test secret")}
	if err := json.NewEncoder(f).Encode(&secret); err != nil {
		return nil, err
	}
	return f, f.Close()
}

func setupListener() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	return l
}

type errorEvent struct {
	err    atomic.Value
	signal chan struct{} // closed after 'err' is populated
}

func (e *errorEvent) Set(err error) {
	if err != nil {
		e.err.Store(err)
	}
	close(e.signal)
}

func (e *errorEvent) Get() error {
	<-e.signal
	err, _ := e.err.Load().(error)
	return err
}

// httpGet makes a blocking request, aborting it if 'abort' is signaled.
func httpGet(addr string, abort *errorEvent) (resp string, err error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var res *http.Response
		if res, err = http.Get(addr); err != nil {
			return
		}
		defer res.Body.Close()
		var blob []byte
		if blob, err = ioutil.ReadAll(res.Body); err != nil {
			return
		}
		if res.StatusCode != 200 {
			err = fmt.Errorf("unexpected status %d", res.StatusCode)
		}
		resp = string(blob)
	}()

	select {
	case <-abort.signal:
		err = abort.Get()
	case <-done:
	}
	return
}
