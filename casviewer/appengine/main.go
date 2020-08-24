// Copyright 2020 The LUCI Authors.
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

package main

import (
	"fmt"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/client/cas"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/router"
)

func main() {
	server.Main(nil, nil, func(srv *server.Server) error {

		srv.Routes.GET("/", router.MiddlewareChain{}, func(ctx *router.Context) {
			// request handler part
			// TODO: retrieve instance from the request URL.
			inst := "projects/chromium-swarm-dev/instances/default_instance"
			// TODO: retrieve digest from the request URL.
			d := "111aa690b079d1503c73812e66136e76a86bc86077ebdac0f72bc7def5986f2f/84"
			// TODO: configure token server host in settings.cfg.
			tHost := "luci-token-server-dev.appspot.com"
			rName := fmt.Sprintf("%s/blobs/%s", inst, d)

			// Application logic begin
			c, _ := cas.NewClient(ctx.Context, inst, tHost, true)
			b, _ := c.ReadBytes(ctx.Context, rName)
			dir := &repb.Directory{}
			_ = proto.Unmarshal(b, dir)
			// Application logic end

			// View part.
			// TODO: render html with the retrieved directory.
			ctx.Writer.Write([]byte("Hello, world. This is CAS Viewer."))
		})

		return nil
	})
}
