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

package git

import (
	"net/http"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/server/auth"
	"golang.org/x/net/context"
)

// ClientFactory creates a Gitiles client.
type ClientFactory func(ctx context.Context, host string) (gitilespb.GitilesClient, error)

var factoryKey = "gitiles client factory key"

// UseFactory installs f into c.
func UseFactory(c context.Context, f ClientFactory) context.Context {
	return context.WithValue(c, &factoryKey, f)
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
var whitelistPublicDomains = stringset.NewFromSlice(
	"chromium.googlesource.com",
)

// AuthenticatedProdClient returns a production Gitiles client.
//
// Currently, it is authenticated as self only for a whitelistPublicDomains.
// For all other repos, the client will not use authentication to avoid
// information leaks.
// TODO(tandrii): fix this per https://crbug.com/796317.
//
// Implements ClientFactory.
func AuthenticatedProdClient(c context.Context, host string) (gitilespb.GitilesClient, error) {
	var t http.RoundTripper
	var err error
	if whitelistPublicDomains.Has(host) {
		t, err = auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
	} else {
		// No scopes if we aren't authenticating.
		t, err = auth.GetRPCTransport(c, auth.NoAuth)
	}
	if err != nil {
		return nil, errors.Annotate(err, "getting RPC Transport").Err()
	}
	return gitiles.NewRESTClient(&http.Client{Transport: t}, host, true)
}

// Client creates a new Gitiles client using the ClientFactory installed in c.
// See also UseFactory.
func Client(c context.Context, host string) (gitilespb.GitilesClient, error) {
	f, ok := c.Value(&factoryKey).(ClientFactory)
	if !ok {
		return nil, errors.New("gitiles client factory is not installed in context")
	}
	return f(c, host)
}
