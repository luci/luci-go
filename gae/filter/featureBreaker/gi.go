// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package featureBreaker

import (
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
)

type infoState struct {
	*state

	info.RawInterface
}

func (g *infoState) ModuleHostname(module, version, instance string) (ret string, err error) {
	err = g.run(func() (err error) {
		ret, err = g.RawInterface.ModuleHostname(module, version, instance)
		return
	})
	return
}

func (g *infoState) ServiceAccount() (ret string, err error) {
	err = g.run(func() (err error) {
		ret, err = g.RawInterface.ServiceAccount()
		return
	})
	return
}

func (g *infoState) Namespace(namespace string) (ret context.Context, err error) {
	err = g.run(func() (err error) {
		ret, err = g.RawInterface.Namespace(namespace)
		return
	})
	return
}

func (g *infoState) AccessToken(scopes ...string) (token string, expiry time.Time, err error) {
	err = g.run(func() (err error) {
		token, expiry, err = g.RawInterface.AccessToken(scopes...)
		return
	})
	return
}

func (g *infoState) PublicCertificates() (ret []info.Certificate, err error) {
	err = g.run(func() (err error) {
		ret, err = g.RawInterface.PublicCertificates()
		return
	})
	return
}

func (g *infoState) SignBytes(bytes []byte) (keyName string, signature []byte, err error) {
	err = g.run(func() (err error) {
		keyName, signature, err = g.RawInterface.SignBytes(bytes)
		return
	})
	return
}

// FilterGI installs a featureBreaker info filter in the context.
func FilterGI(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return info.AddFilters(c, func(ic context.Context, i info.RawInterface) info.RawInterface {
		return &infoState{state, i}
	}), state
}
