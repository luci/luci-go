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

package endpoints

import (
	"sync"

	"go.chromium.org/luci/appengine/gaeauth/server/gaesigner"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/server/router"
)

// Services is a set of support services used by AppEngine Classic Coordinator
// endpoints.
//
// Each instance is valid for a single request, but can be re-used throughout
// that request. This is advised, as the Services instance may optionally cache
// values.
//
// Services methods are goroutine-safe.
type Services interface {
	coordinator.ConfigProvider
}

// ProdService is an instance-global configuration for production
// Coordinator services. A zero-value struct should be used.
//
// It can be installed via middleware using its Base method.
// This also fulfills the publisher ClientFactory interface.
type ProdService struct {
}

// Base is Middleware used by Coordinator services.
//
// It installs a production Services instance into the Context.
func (svc *ProdService) Base(c *router.Context, next router.Handler) {
	services := prodServicesInst{
		ProdService: svc,
	}

	c.Context = coordinator.WithConfigProvider(c.Context, &services)
	c.Context = WithServices(c.Context, &services)
	next(c)
}

// prodServicesInst is a Service exposing production faciliites. A unique
// instance is bound to each each request.
type prodServicesInst struct {
	*ProdService
	sync.Mutex

	// LUCIConfigProvider satisfies the ConfigProvider interface requirement.
	coordinator.LUCIConfigProvider

	// signer is the signer instance to use.
	signer gaesigner.Signer
}
