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

package count

import (
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
)

// InfoCounter is the counter object for the GlobalInfo service.
type InfoCounter struct {
	AppID                  Entry
	FullyQualifiedAppID    Entry
	GetNamespace           Entry
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

	gi info.RawInterface
}

var _ info.RawInterface = (*infoCounter)(nil)

func (g *infoCounter) AppID() string {
	_ = g.c.AppID.up()
	return g.gi.AppID()
}

func (g *infoCounter) FullyQualifiedAppID() string {
	_ = g.c.FullyQualifiedAppID.up()
	return g.gi.FullyQualifiedAppID()
}

func (g *infoCounter) GetNamespace() string {
	_ = g.c.GetNamespace.up()
	return g.gi.GetNamespace()
}

func (g *infoCounter) Datacenter() string {
	_ = g.c.Datacenter.up()
	return g.gi.Datacenter()
}

func (g *infoCounter) DefaultVersionHostname() string {
	_ = g.c.DefaultVersionHostname.up()
	return g.gi.DefaultVersionHostname()
}

func (g *infoCounter) InstanceID() string {
	_ = g.c.InstanceID.up()
	return g.gi.InstanceID()
}

func (g *infoCounter) IsDevAppServer() bool {
	_ = g.c.IsDevAppServer.up()
	return g.gi.IsDevAppServer()
}

func (g *infoCounter) IsOverQuota(err error) bool {
	_ = g.c.IsOverQuota.up()
	return g.gi.IsOverQuota(err)
}

func (g *infoCounter) IsTimeoutError(err error) bool {
	_ = g.c.IsTimeoutError.up()
	return g.gi.IsTimeoutError(err)
}

func (g *infoCounter) ModuleHostname(module, version, instance string) (string, error) {
	ret, err := g.gi.ModuleHostname(module, version, instance)
	return ret, g.c.ModuleHostname.up(err)
}

func (g *infoCounter) ModuleName() string {
	_ = g.c.ModuleName.up()
	return g.gi.ModuleName()
}

func (g *infoCounter) RequestID() string {
	_ = g.c.RequestID.up()
	return g.gi.RequestID()
}

func (g *infoCounter) ServerSoftware() string {
	_ = g.c.ServerSoftware.up()
	return g.gi.ServerSoftware()
}

func (g *infoCounter) ServiceAccount() (string, error) {
	ret, err := g.gi.ServiceAccount()
	return ret, g.c.ServiceAccount.up(err)
}

func (g *infoCounter) VersionID() string {
	_ = g.c.VersionID.up()
	return g.gi.VersionID()
}

func (g *infoCounter) Namespace(namespace string) (c context.Context, err error) {
	c, err = g.gi.Namespace(namespace)
	g.c.Namespace.up(err)
	return
}

func (g *infoCounter) AccessToken(scopes ...string) (string, time.Time, error) {
	token, expiry, err := g.gi.AccessToken(scopes...)
	return token, expiry, g.c.AccessToken.up(err)
}

func (g *infoCounter) PublicCertificates() ([]info.Certificate, error) {
	ret, err := g.gi.PublicCertificates()
	return ret, g.c.PublicCertificates.up(err)
}

func (g *infoCounter) SignBytes(bytes []byte) (string, []byte, error) {
	keyName, signature, err := g.gi.SignBytes(bytes)
	return keyName, signature, g.c.SignBytes.up(err)
}

func (g *infoCounter) GetTestable() info.Testable {
	return g.gi.GetTestable()
}

// FilterGI installs a counter GlobalInfo filter in the context.
func FilterGI(c context.Context) (context.Context, *InfoCounter) {
	state := &InfoCounter{}
	return info.AddFilters(c, func(ic context.Context, gi info.RawInterface) info.RawInterface {
		return &infoCounter{state, gi}
	}), state
}
