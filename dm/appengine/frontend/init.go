// Copyright 2015 The LUCI Authors.
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

package frontend

import (
	"net/http"

	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/dm/appengine/deps"
	"github.com/luci/luci-go/dm/appengine/distributor"
	"github.com/luci/luci-go/dm/appengine/distributor/jobsim"
	"github.com/luci/luci-go/dm/appengine/distributor/swarming/v1"
	"github.com/luci/luci-go/dm/appengine/mutate"
	"github.com/luci/luci-go/grpc/discovery"
	"github.com/luci/luci-go/grpc/prpc"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/tumble"
)

func init() {
	tmb := tumble.Service{}

	distributors := distributor.FactoryMap{}
	jobsim.AddFactory(distributors)
	swarming.AddFactory(distributors)

	reg := distributor.NewRegistry(distributors, mutate.FinishExecutionFn)

	basemw := gaemiddleware.BaseProd().Extend(func(c *router.Context, next router.Handler) {
		c.Context = distributor.WithRegistry(c.Context, reg)
		next(c)
	})

	r := router.New()

	svr := prpc.Server{}
	deps.RegisterDepsServer(&svr)
	discovery.Enable(&svr)

	distributor.InstallHandlers(r, basemw)
	svr.InstallHandlers(r, basemw)
	tmb.InstallHandlers(r, basemw)

	// TODO(iannucci): We can probably use gaemiddleware.InstallHandlers here,
	// since various framework-level hooks (settings pages, tsmon callbacks), do
	// not need a distributor registry.
	gaemiddleware.InstallHandlersWithMiddleware(r, basemw)

	http.Handle("/", r)
}
