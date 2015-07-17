// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"golang.org/x/net/context"
	"time"

	"infra/gae/libs/gae"
)

// GICounter is the counter object for the GlobalInfo service.
type GICounter struct {
	AppID                  Entry
	Datacenter             Entry
	DefaultVersionHostname Entry
	InstanceID             Entry
	IsDevAppServer         Entry
	IsOverQuota            Entry
	IsTimeoutError         Entry
	ModuleHostname         Entry
	ModuleName             Entry
	RequestID              Entry
	ServerSoftware         Entry
	ServiceAccount         Entry
	VersionID              Entry
	Namespace              Entry
	AccessToken            Entry
	PublicCertificates     Entry
	SignBytes              Entry
}

type giCounter struct {
	c *GICounter

	gi gae.GlobalInfo
}

var _ gae.GlobalInfo = (*giCounter)(nil)

func (g *giCounter) AppID() string {
	g.c.AppID.up()
	return g.gi.AppID()
}

func (g *giCounter) Datacenter() string {
	g.c.Datacenter.up()
	return g.gi.Datacenter()
}

func (g *giCounter) DefaultVersionHostname() string {
	g.c.DefaultVersionHostname.up()
	return g.gi.DefaultVersionHostname()
}

func (g *giCounter) InstanceID() string {
	g.c.InstanceID.up()
	return g.gi.InstanceID()
}

func (g *giCounter) IsDevAppServer() bool {
	g.c.IsDevAppServer.up()
	return g.gi.IsDevAppServer()
}

func (g *giCounter) IsOverQuota(err error) bool {
	g.c.IsOverQuota.up()
	return g.gi.IsOverQuota(err)
}

func (g *giCounter) IsTimeoutError(err error) bool {
	g.c.IsTimeoutError.up()
	return g.gi.IsTimeoutError(err)
}

func (g *giCounter) ModuleHostname(module, version, instance string) (string, error) {
	ret, err := g.gi.ModuleHostname(module, version, instance)
	return ret, g.c.ModuleHostname.up(err)
}

func (g *giCounter) ModuleName() string {
	g.c.ModuleName.up()
	return g.gi.ModuleName()
}

func (g *giCounter) RequestID() string {
	g.c.RequestID.up()
	return g.gi.RequestID()
}

func (g *giCounter) ServerSoftware() string {
	g.c.ServerSoftware.up()
	return g.gi.ServerSoftware()
}

func (g *giCounter) ServiceAccount() (string, error) {
	ret, err := g.gi.ServiceAccount()
	return ret, g.c.ServiceAccount.up(err)
}

func (g *giCounter) VersionID() string {
	g.c.VersionID.up()
	return g.gi.VersionID()
}

func (g *giCounter) Namespace(namespace string) (context.Context, error) {
	ret, err := g.gi.Namespace(namespace)
	return ret, g.c.Namespace.up(err)
}

func (g *giCounter) AccessToken(scopes ...string) (token string, expiry time.Time, err error) {
	token, expiry, err = g.gi.AccessToken(scopes...)
	g.c.AccessToken.up(err)
	return
}

func (g *giCounter) PublicCertificates() ([]gae.GICertificate, error) {
	ret, err := g.gi.PublicCertificates()
	return ret, g.c.PublicCertificates.up(err)
}

func (g *giCounter) SignBytes(bytes []byte) (keyName string, signature []byte, err error) {
	keyName, signature, err = g.gi.SignBytes(bytes)
	g.c.SignBytes.up(err)
	return
}

// FilterGI installs a counter GlobalInfo filter in the context.
func FilterGI(c context.Context) (context.Context, *GICounter) {
	state := &GICounter{}
	return gae.AddGIFilters(c, func(ic context.Context, gi gae.GlobalInfo) gae.GlobalInfo {
		return &giCounter{state, gi}
	}), state
}
