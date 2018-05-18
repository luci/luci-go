// Copyright 2018 The LUCI Authors.
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

package common

import (
	"errors"
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/auth"
)

var gerritClientFactoryKey = "gerrit client factory"

// GerritFactory creates a Gerrit client.
type GerritFactory func(c context.Context, host string) (gerritpb.GerritClient, error)

// WithGerritFactory returns a context with the Gerrit client factory.
func WithGerritFactory(c context.Context, factory GerritFactory) context.Context {
	return context.WithValue(c, &gerritClientFactoryKey, factory)
}

// GetGerritFactory returns the GerritFactory installed into c using
// WithGerritFactory if it is there, otherwise returns nil.
func GetGerritFactory(c context.Context) GerritFactory {
	if f, ok := c.Value(&gerritClientFactoryKey).(GerritFactory); ok {
		return f
	}
	return nil
}

// CreateGerritClient creates a Gerrit client using the factory installed into
// c using WithGerritFactory.
func CreateGerritClient(c context.Context, host string) (gerritpb.GerritClient, error) {
	factory := GetGerritFactory(c)
	if factory == nil {
		return nil, errors.New("no Gerrit client factory")
	}
	return factory(c, host)
}

// TODO(tandrii): remove the following per https://crbug.com/796317.
// Until Milo properly supports ACLs for blamelists, we have a hack; if the git
// repo being log'd is in this list, use `auth.AsSelf`. Otherwise use
// `auth.Anonymous`.
//
// The reason to do this is that we currently do blamelist calculation in the
// backend, so we can't accurately determine if the requesting user has access
// to these repos or not. For now, we use this whitelist to indicate domains
// that we know have full public read-access so that we can use milo's
// credentials (instead of anonymous) in order to avoid hitting gitiles'
// anonymous quota limits.
var whitelistPublicHosts = stringset.NewFromSlice(
	"chromium",
	"fuchsia",
)

func isPublicHost(host string) bool {
	const gs = ".googlesource.com"
	if !strings.HasSuffix(host, gs) {
		return false
	}

	host = strings.TrimSuffix(host, gs)
	host = strings.TrimSuffix(host, "-review")
	return whitelistPublicHosts.Has(host)
}

// Transport returns an HTTP transport to be used for the given host.
//
// Currently, it is authenticated as self only for a whitelistPublicHosts.
// For all other repos, the transport will not use authentication to avoid
// information leaks.
// TODO(tandrii): fix this per https://crbug.com/796317.
func Transport(c context.Context, host string) (transport http.RoundTripper, authenticated bool, err error) {
	if isPublicHost(host) {
		transport, err = auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
		authenticated = true
	} else {
		transport, err = auth.GetRPCTransport(c, auth.NoAuth)
		authenticated = false
	}
	return
}
