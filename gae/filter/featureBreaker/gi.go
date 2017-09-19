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

package featureBreaker

import (
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
)

type infoState struct {
	*state

	c context.Context
	info.RawInterface
}

func (g *infoState) ModuleHostname(module, version, instance string) (ret string, err error) {
	err = g.run(g.c, func() (err error) {
		ret, err = g.RawInterface.ModuleHostname(module, version, instance)
		return
	})
	return
}

func (g *infoState) ServiceAccount() (ret string, err error) {
	err = g.run(g.c, func() (err error) {
		ret, err = g.RawInterface.ServiceAccount()
		return
	})
	return
}

func (g *infoState) Namespace(namespace string) (c context.Context, err error) {
	err = g.run(g.c, func() (err error) {
		c, err = g.RawInterface.Namespace(namespace)
		return
	})
	return
}

func (g *infoState) AccessToken(scopes ...string) (token string, expiry time.Time, err error) {
	err = g.run(g.c, func() (err error) {
		token, expiry, err = g.RawInterface.AccessToken(scopes...)
		return
	})
	return
}

func (g *infoState) PublicCertificates() (ret []info.Certificate, err error) {
	err = g.run(g.c, func() (err error) {
		ret, err = g.RawInterface.PublicCertificates()
		return
	})
	return
}

func (g *infoState) SignBytes(bytes []byte) (keyName string, signature []byte, err error) {
	err = g.run(g.c, func() (err error) {
		keyName, signature, err = g.RawInterface.SignBytes(bytes)
		return
	})
	return
}

// FilterGI installs a featureBreaker info filter in the context.
func FilterGI(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return info.AddFilters(c, func(ic context.Context, i info.RawInterface) info.RawInterface {
		return &infoState{state, ic, i}
	}), state
}
