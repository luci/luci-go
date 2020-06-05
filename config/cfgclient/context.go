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

package cfgclient

import (
	"context"
	"errors"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/erroring"
)

var contextKey = "go.chromium.org/luci/config/cfgclient"

// Use installs a client implementation into the context.
//
// Primarily used by the framework code to setup an appropriate implementation.
// Can be used in unit tests to install a testing implementation.
//
// Do not call it yourself outside of unit tests.
func Use(ctx context.Context, client config.Interface) context.Context {
	return context.WithValue(ctx, &contextKey, client)
}

// Client returns a client to access the LUCI Config service.
//
// If there's no client in the context, returns a client that fails all calls
// with an error.
func Client(ctx context.Context) config.Interface {
	if impl, _ := ctx.Value(&contextKey).(config.Interface); impl != nil {
		return impl
	}
	return erroring.New(errors.New("LUCI Config client is not available in the context"))
}
