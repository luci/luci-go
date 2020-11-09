// Copyright 2020 The LUCI Authors.
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

package gerrit

import (
	"context"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"

	luciauth "go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
)

// factory knows how to construct Gerrit Clients.
//
// CV must use project-scoped credentials, but not every project has configured
// project-scoped service account (PSSA). Worse, some projects configured PSSA
// before CQDaemon supported PSSA. Hence, when CQDaemon gained support for PSSA,
// CQDaemon had to blocklist these projects to avoid breakage.  The alternative
// and legacy authentication is based on per GerritHost auth tokens from
// ~/.netrc shared by all LUCI projects. CQDaemon logic is roughly:
//
//   if project in blocklist:
//     return use_legacy_netrc
//   try:
//     token = token_server.MintToken(project)
//   except 404: # not configured
//     return use_legacy_netrc
//   return use_pssa(token)
//
// For smooth migration from CQDaemon to CV, CV re-implements the same logic.
//
// Caveat: for smooth migration of other LUCI services in Go to PSSA,
// auth.GetRPCTransport(ctx, auth.AsProject, ...) helpfully and transparently
// defaults to auth.AsSelf if LUCI project doesn't have PSSA configured.
// Thus CV can't rely on the above method as is.
//
// TODO(tandrii): move hardcoded projects blocklist in CQDaemon to LUCI
// config for both CV and CQDaemon to agree on source of truth.
type factory struct {
	mutex         sync.RWMutex
	pssaBlocklist stringset.Set
	legacyCache   map[string]*oauth2.Token // caches tokens and lack thereof

	mockMintProjectToken func(context.Context, auth.ProjectTokenParams) (*auth.Token, error)
}

func newFactory() *factory {
	return &factory{
		// Current list is <30 and it won't grow.
		pssaBlocklist: stringset.New(50),
		// CV supports <20 legacy hosts. New ones won't be added.
		legacyCache: make(map[string]*oauth2.Token, 20),
	}
}

func (f *factory) makeClient(ctx context.Context, gerritHost, luciProject string) (Client, error) {
	t, err := f.transport(ctx, gerritHost, luciProject)
	if err != nil {
		return nil, err
	}
	return gerrit.NewRESTClient(&http.Client{Transport: t}, gerritHost, true)
}

func (f *factory) transport(ctx context.Context, gerritHost, luciProject string) (http.RoundTripper, error) {
	// Do what auth.GetRPCTransport(ctx, auth.AsProject, ...) would do,
	// except obey pssaBlocklist and default to legacy ~/.netrc creds.
	// See factory doc for more details.
	baseTransport, err := auth.GetRPCTransport(ctx, auth.NoAuth)
	if err != nil {
		return nil, err
	}
	return luciauth.NewModifyingTransport(baseTransport, func(req *http.Request) error {
		tok, err := f.token(ctx, gerritHost, luciProject)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", tok.TokenType+" "+tok.AccessToken)
		return nil
	}), nil
}

func (f *factory) token(ctx context.Context, gerritHost, luciProject string) (*oauth2.Token, error) {
	f.mutex.RLock()
	if !f.pssaBlocklist.Has(luciProject) {
		f.mutex.RUnlock()

		req := auth.ProjectTokenParams{
			MinTTL:      2 * time.Minute,
			LuciProject: luciProject,
			OAuthScopes: []string{gerrit.OAuthScope},
		}
		mintToken := auth.MintProjectToken
		if f.mockMintProjectToken != nil {
			mintToken = f.mockMintProjectToken
		}
		switch token, err := mintToken(ctx, req); {
		case err != nil:
			return nil, err
		case token != nil:
			return &oauth2.Token{
				AccessToken: token.Token,
				TokenType:   "Bearer",
			}, nil
		}

		// PSSA not configured.
		f.mutex.RLock()
	}
	switch t, err := f.loadLegacyAndRUnlock(ctx, gerritHost); {
	case err != nil:
		return nil, err
	case t.AccessToken == "":
		return nil, errors.Reason("No legacy credentials for host %q", gerritHost).Err()
	default:
		return t, nil
	}
}

func (f *factory) loadLegacyAndRUnlock(ctx context.Context, gerritHost string) (*oauth2.Token, error) {
	// Load legacy from cache.
	t, ok := f.legacyCache[gerritHost]
	f.mutex.RUnlock()
	if ok && t.Expiry.After(clock.Now(ctx)) {
		return t, nil
	}

	// Load legacy from datastore.
	t = &oauth2.Token{TokenType: "Basic"}
	nt := netrcToken{GerritHost: gerritHost}
	switch err := datastore.Get(ctx, &nt); {
	case err == nil:
		t.AccessToken = nt.AccessToken
	case err != datastore.ErrNoSuchEntity:
		return nil, errors.Annotate(err, "failed to get legacy creds").Tag(transient.Tag).Err()
	}
	t.Expiry = clock.Now(ctx).Add(cacheLegacyDuration)

	f.mutex.Lock()
	f.legacyCache[gerritHost] = t
	f.mutex.Unlock()
	return t, nil
}

// cacheLegacyDuration defines lifetime
const cacheLegacyDuration = 10 * time.Minute

// netrcToken stores ~/.netrc access tokens of CQDaemon.
type netrcToken struct {
	GerritHost  string `gae:"$id"`
	AccessToken string `gae:",noindex"`
}

// SaveLegacyNetrcToken creates or updates legacy netrc token.
func SaveLegacyNetrcToken(ctx context.Context, host, token string) error {
	err := datastore.Put(ctx, &netrcToken{host, token})
	return errors.Annotate(err, "failed to save legacy netrc token").Err()
}
