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

package prod

import (
	"time"

	"go.chromium.org/gae/service/info"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

// useGI adds a gae.GlobalInfo implementation to context, accessible
// by gae.GetGI(c)
func useGI(usrCtx context.Context) context.Context {
	probeCache := getProbeCache(usrCtx)
	if probeCache == nil {
		usrCtx = withProbeCache(usrCtx, probe(getAEContext(usrCtx)))
	}

	return info.SetFactory(usrCtx, func(ci context.Context) info.RawInterface {
		return giImpl{ci, getAEContext(ci)}
	})
}

type giImpl struct {
	usrCtx context.Context
	aeCtx  context.Context
}

func (g giImpl) AccessToken(scopes ...string) (token string, expiry time.Time, err error) {
	return appengine.AccessToken(g.aeCtx, scopes...)
}
func (g giImpl) AppID() string {
	return appengine.AppID(g.aeCtx)
}
func (g giImpl) FullyQualifiedAppID() string {
	return getProbeCache(g.usrCtx).fqaid
}
func (g giImpl) GetNamespace() string {
	return getProbeCache(g.usrCtx).namespace
}
func (g giImpl) Datacenter() string {
	return appengine.Datacenter(g.aeCtx)
}
func (g giImpl) DefaultVersionHostname() string {
	return appengine.DefaultVersionHostname(g.aeCtx)
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
	return appengine.ModuleHostname(g.aeCtx, module, version, instance)
}
func (g giImpl) ModuleName() (name string) {
	return appengine.ModuleName(g.aeCtx)
}
func (g giImpl) Namespace(namespace string) (context.Context, error) {
	c := g.usrCtx

	pc := *getProbeCache(c)
	if pc.namespace == namespace {
		// Already using this namespace.
		return c, nil
	}
	pc.namespace = namespace

	// Apply the namespace to our retained GAE Contexts.
	var err error
	ps := getProdState(c)

	// Apply to current GAE Context.
	if ps.ctx, err = appengine.Namespace(ps.ctx, namespace); err != nil {
		return c, err
	}

	// Apply to non-transactional Context. Since the previous one applied
	// successfully, this must succeed.
	ps.noTxnCtx, err = appengine.Namespace(ps.noTxnCtx, namespace)
	if err != nil {
		panic(err)
	}

	// Update our user Context with the new namespace-imbued objects.
	c = withProbeCache(c, &pc)
	c = withProdState(c, ps)
	return c, nil
}
func (g giImpl) PublicCertificates() ([]info.Certificate, error) {
	certs, err := appengine.PublicCertificates(g.aeCtx)
	if err != nil {
		return nil, err
	}
	ret := make([]info.Certificate, len(certs))
	for i, c := range certs {
		ret[i] = info.Certificate(c)
	}
	return ret, nil
}
func (g giImpl) RequestID() string {
	return appengine.RequestID(g.aeCtx)
}
func (g giImpl) ServerSoftware() string {
	return appengine.ServerSoftware()
}
func (g giImpl) ServiceAccount() (string, error) {
	if appengine.IsDevAppServer() {
		// On devserver ServiceAccount returns empty string, but AccessToken works.
		// We use it to grab developer's email.
		return developerAccount(g.aeCtx)
	}
	return appengine.ServiceAccount(g.aeCtx)
}
func (g giImpl) SignBytes(bytes []byte) (keyName string, signature []byte, err error) {
	return appengine.SignBytes(g.aeCtx, bytes)
}
func (g giImpl) VersionID() string {
	return appengine.VersionID(g.aeCtx)
}

func (g giImpl) GetTestable() info.Testable { return nil }

type infoProbeCache struct {
	namespace string
	fqaid     string
}

func probe(aeCtx context.Context) *infoProbeCache {
	probeKey := datastore.NewKey(aeCtx, "Kind", "id", 0, nil)
	ipb := infoProbeCache{
		fqaid:     probeKey.AppID(),
		namespace: probeKey.Namespace(),
	}
	return &ipb
}

func getProbeCache(c context.Context) *infoProbeCache {
	if pc, ok := c.Value(&probeCacheKey).(*infoProbeCache); ok {
		return pc
	}
	return nil
}

func withProbeCache(c context.Context, pc *infoProbeCache) context.Context {
	return context.WithValue(c, &probeCacheKey, pc)
}
