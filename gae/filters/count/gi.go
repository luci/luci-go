// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
)

// InfoCounter is the counter object for the GlobalInfo service.
type InfoCounter struct {
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

type infoCounter struct {
	c *InfoCounter

	gi info.Interface
}

var _ info.Interface = (*infoCounter)(nil)

func (g *infoCounter) AppID() string {
	g.c.AppID.up()
	return g.gi.AppID()
}

func (g *infoCounter) Datacenter() string {
	g.c.Datacenter.up()
	return g.gi.Datacenter()
}

func (g *infoCounter) DefaultVersionHostname() string {
	g.c.DefaultVersionHostname.up()
	return g.gi.DefaultVersionHostname()
}

func (g *infoCounter) InstanceID() string {
	g.c.InstanceID.up()
	return g.gi.InstanceID()
}

func (g *infoCounter) IsDevAppServer() bool {
	g.c.IsDevAppServer.up()
	return g.gi.IsDevAppServer()
}

func (g *infoCounter) IsOverQuota(err error) bool {
	g.c.IsOverQuota.up()
	return g.gi.IsOverQuota(err)
}

func (g *infoCounter) IsTimeoutError(err error) bool {
	g.c.IsTimeoutError.up()
	return g.gi.IsTimeoutError(err)
}

func (g *infoCounter) ModuleHostname(module, version, instance string) (string, error) {
	ret, err := g.gi.ModuleHostname(module, version, instance)
	return ret, g.c.ModuleHostname.up(err)
}

func (g *infoCounter) ModuleName() string {
	g.c.ModuleName.up()
	return g.gi.ModuleName()
}

func (g *infoCounter) RequestID() string {
	g.c.RequestID.up()
	return g.gi.RequestID()
}

func (g *infoCounter) ServerSoftware() string {
	g.c.ServerSoftware.up()
	return g.gi.ServerSoftware()
}

func (g *infoCounter) ServiceAccount() (string, error) {
	ret, err := g.gi.ServiceAccount()
	return ret, g.c.ServiceAccount.up(err)
}

func (g *infoCounter) VersionID() string {
	g.c.VersionID.up()
	return g.gi.VersionID()
}

func (g *infoCounter) Namespace(namespace string) (context.Context, error) {
	ret, err := g.gi.Namespace(namespace)
	return ret, g.c.Namespace.up(err)
}

func (g *infoCounter) AccessToken(scopes ...string) (token string, expiry time.Time, err error) {
	token, expiry, err = g.gi.AccessToken(scopes...)
	g.c.AccessToken.up(err)
	return
}

func (g *infoCounter) PublicCertificates() ([]info.Certificate, error) {
	ret, err := g.gi.PublicCertificates()
	return ret, g.c.PublicCertificates.up(err)
}

func (g *infoCounter) SignBytes(bytes []byte) (keyName string, signature []byte, err error) {
	keyName, signature, err = g.gi.SignBytes(bytes)
	g.c.SignBytes.up(err)
	return
}

// FilterGI installs a counter GlobalInfo filter in the context.
func FilterGI(c context.Context) (context.Context, *InfoCounter) {
	state := &InfoCounter{}
	return info.AddFilters(c, func(ic context.Context, gi info.Interface) info.Interface {
		return &infoCounter{state, gi}
	}), state
}
