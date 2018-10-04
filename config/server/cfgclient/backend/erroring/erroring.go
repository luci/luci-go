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

// Package erroring implements config.Backend that simply returns an error.
//
// May be handy as a placeholder in case some more useful implementation is not
// available.
package erroring

import (
	"context"
	"net/url"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/erroring"
	"go.chromium.org/luci/config/server/cfgclient/backend"
)

// New produces backend.B instance that returns the given error for all calls.
//
// Panics if given err is nil.
func New(err error) backend.B {
	if err == nil {
		panic("the error must not be nil")
	}
	return erroringImpl{err}
}

type erroringImpl struct {
	err error
}

func (e erroringImpl) ServiceURL(context.Context) url.URL {
	return url.URL{
		Scheme: "error",
	}
}

func (e erroringImpl) Get(c context.Context, configSet config.Set, path string, p backend.Params) (*config.Config, error) {
	return nil, e.err
}

func (e erroringImpl) GetAll(c context.Context, t backend.GetAllTarget, path string, p backend.Params) (
	[]*config.Config, error) {

	return nil, e.err
}

func (e erroringImpl) GetConfigInterface(c context.Context, a backend.Authority) config.Interface {
	return erroring.New(e.err)
}
