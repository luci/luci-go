// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
)

type infoState struct {
	*state

	info.Interface
}

func (g *infoState) ModuleHostname(module, version, instance string) (ret string, err error) {
	err = g.run(func() (err error) {
		ret, err = g.Interface.ModuleHostname(module, version, instance)
		return
	})
	return
}

func (g *infoState) ServiceAccount() (ret string, err error) {
	err = g.run(func() (err error) {
		ret, err = g.Interface.ServiceAccount()
		return
	})
	return
}

func (g *infoState) Namespace(namespace string) (ret context.Context, err error) {
	err = g.run(func() (err error) {
		ret, err = g.Interface.Namespace(namespace)
		return
	})
	return
}

func (g *infoState) MustNamespace(namespace string) context.Context {
	ret, err := g.Namespace(namespace)
	if err != nil {
		panic(err)
	}
	return ret
}

func (g *infoState) AccessToken(scopes ...string) (token string, expiry time.Time, err error) {
	err = g.run(func() (err error) {
		token, expiry, err = g.Interface.AccessToken(scopes...)
		return
	})
	return
}

func (g *infoState) PublicCertificates() (ret []info.Certificate, err error) {
	err = g.run(func() (err error) {
		ret, err = g.Interface.PublicCertificates()
		return
	})
	return
}

func (g *infoState) SignBytes(bytes []byte) (keyName string, signature []byte, err error) {
	err = g.run(func() (err error) {
		keyName, signature, err = g.Interface.SignBytes(bytes)
		return
	})
	return
}

// FilterGI installs a counter info filter in the context.
func FilterGI(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return info.AddFilters(c, func(ic context.Context, i info.Interface) info.Interface {
		return &infoState{state, i}
	}), state
}
