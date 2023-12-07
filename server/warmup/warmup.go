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

// Package warmup allows to register hooks executed during the server warmup.
//
// All registered hooks should be optional. The warmup stage can be skipped.
package warmup

import (
	"context"
	"net/http"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/router"
)

// Callback will be called during warmup.
type Callback func(c context.Context) error

var state struct {
	sync.Mutex
	callbacks []callbackWithName
}

type callbackWithName struct {
	Callback
	name string
}

// Register adds a callback called during warmup.
func Register(name string, cb Callback) {
	if name == "" {
		panic("warmup callback name is required")
	}
	state.Lock()
	defer state.Unlock()
	state.callbacks = append(state.callbacks, callbackWithName{cb, name})
}

// Warmup executes all registered warmup callbacks, sequentially.
//
// Doesn't abort on individual callback errors, just collects and returns them
// all.
func Warmup(c context.Context) error {
	state.Lock()
	defer state.Unlock()

	var merr errors.MultiError
	for _, cb := range state.callbacks {
		logging.Infof(c, "Warming up %q", cb.name)
		if err := cb.Callback(c); err != nil {
			logging.Errorf(c, "Error when warming up %q: %s", cb.name, err)
			merr = append(merr, err)
		}
	}

	logging.Infof(c, "Finished warming up")
	if len(merr) == 0 {
		return nil
	}
	return merr
}

// InstallHandlersDeprecated installs HTTP handlers for warmup /_ah/* routes.
//
// It is deprecated. Do not use. On GAEv1 it is called by the framework code.
// On GAEv2 or on GKE use NewModuleFromFlags() and register the warmup module
// when starting the server.
func InstallHandlersDeprecated(r *router.Router, base router.MiddlewareChain) {
	r.GET("/_ah/warmup", base, httpHandler)
	r.GET("/_ah/start", base, httpHandler)
}

func httpHandler(c *router.Context) {
	status := http.StatusOK
	if err := Warmup(c.Request.Context()); err != nil {
		status = http.StatusInternalServerError
	}
	c.Writer.WriteHeader(status)
}
