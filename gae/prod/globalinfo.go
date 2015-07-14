// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"time"

	"golang.org/x/net/context"

	"infra/gae/libs/gae"

	"google.golang.org/appengine"
)

// useGI adds a gae.GlobalInfo implementation to context, accessible
// by gae.GetGI(c)
func useGI(c context.Context) context.Context {
	return gae.SetGIFactory(c, func(ci context.Context) gae.GlobalInfo {
		return giImpl{ci}
	})
}

type giImpl struct{ context.Context }

func (g giImpl) AccessToken(scopes ...string) (token string, expiry time.Time, err error) {
	return appengine.AccessToken(g, scopes...)
}
func (g giImpl) AppID() string {
	return appengine.AppID(g)
}
func (g giImpl) Datacenter() string {
	return appengine.Datacenter(g)
}
func (g giImpl) DefaultVersionHostname() string {
	return appengine.DefaultVersionHostname(g)
}
func (g giImpl) InstanceID() string {
	return appengine.InstanceID()
}
func (g giImpl) IsDevAppServer() bool {
	return appengine.IsDevAppServer()
}
func (g giImpl) IsOverQuota(err error) bool {
	return appengine.IsOverQuota(err)
}
func (g giImpl) IsTimeoutError(err error) bool {
	return appengine.IsTimeoutError(err)
}
func (g giImpl) ModuleHostname(module, version, instance string) (string, error) {
	return appengine.ModuleHostname(g, module, version, instance)
}
func (g giImpl) ModuleName() (name string) {
	return appengine.ModuleName(g)
}
func (g giImpl) Namespace(namespace string) (context.Context, error) {
	return appengine.Namespace(g, namespace)
}
func (g giImpl) PublicCertificates() ([]gae.GICertificate, error) {
	certs, err := appengine.PublicCertificates(g)
	if err != nil {
		return nil, err
	}
	ret := make([]gae.GICertificate, len(certs))
	for i, c := range certs {
		ret[i] = (gae.GICertificate)(c)
	}
	return ret, nil
}
func (g giImpl) RequestID() string {
	return appengine.RequestID(g)
}
func (g giImpl) ServerSoftware() string {
	return appengine.ServerSoftware()
}
func (g giImpl) ServiceAccount() (string, error) {
	return appengine.ServiceAccount(g)
}
func (g giImpl) SignBytes(bytes []byte) (keyName string, signature []byte, err error) {
	return appengine.SignBytes(g, bytes)
}
func (g giImpl) VersionID() string {
	return appengine.VersionID(g)
}
