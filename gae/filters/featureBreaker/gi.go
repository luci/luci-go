// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"golang.org/x/net/context"
	"time"

	"infra/gae/libs/gae"
)

type giState struct {
	*state

	gae.GlobalInfo
}

func (g *giState) ModuleHostname(module, version, instance string) (ret string, err error) {
	err = g.run(func() (err error) {
		ret, err = g.GlobalInfo.ModuleHostname(module, version, instance)
		return
	})
	return
}

func (g *giState) ServiceAccount() (ret string, err error) {
	err = g.run(func() (err error) {
		ret, err = g.GlobalInfo.ServiceAccount()
		return
	})
	return
}

func (g *giState) Namespace(namespace string) (ret context.Context, err error) {
	err = g.run(func() (err error) {
		ret, err = g.GlobalInfo.Namespace(namespace)
		return
	})
	return
}

func (g *giState) AccessToken(scopes ...string) (token string, expiry time.Time, err error) {
	err = g.run(func() (err error) {
		token, expiry, err = g.GlobalInfo.AccessToken(scopes...)
		return
	})
	return
}

func (g *giState) PublicCertificates() (ret []gae.GICertificate, err error) {
	err = g.run(func() (err error) {
		ret, err = g.GlobalInfo.PublicCertificates()
		return
	})
	return
}

func (g *giState) SignBytes(bytes []byte) (keyName string, signature []byte, err error) {
	err = g.run(func() (err error) {
		keyName, signature, err = g.GlobalInfo.SignBytes(bytes)
		return
	})
	return
}

// FilterGI installs a counter GlobalInfo filter in the context.
func FilterGI(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return gae.AddGIFilters(c, func(ic context.Context, gi gae.GlobalInfo) gae.GlobalInfo {
		return &giState{state, gi}
	}), state
}
