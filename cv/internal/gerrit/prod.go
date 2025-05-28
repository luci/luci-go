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
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	luciauth "go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
)

// prodFactory knows how to construct Gerrit clients and hop over Gerrit
// mirrors.
type prodFactory struct {
	baseTransport http.RoundTripper

	mirrorHostPrefixes []string

	mockMintProjectToken func(context.Context, auth.ProjectTokenParams) (*auth.Token, error)
}

var errEmptyProjectToken = errors.New("crbug/824492: Project token is empty")

func newProd(ctx context.Context, mirrorHostPrefixes ...string) (*prodFactory, error) {
	t, err := auth.GetRPCTransport(ctx, auth.NoAuth)
	if err != nil {
		return nil, err
	}
	return &prodFactory{
		baseTransport:      t,
		mirrorHostPrefixes: mirrorHostPrefixes,
	}, nil
}

// MakeMirrorIterator implements Factory.
func (p *prodFactory) MakeMirrorIterator(ctx context.Context) *MirrorIterator {
	return newMirrorIterator(ctx, p.mirrorHostPrefixes...)
}

// MakeClient implements Factory.
func (f *prodFactory) MakeClient(ctx context.Context, gerritHost, luciProject string) (Client, error) {
	if strings.ContainsRune(luciProject, '.') {
		return nil, fmt.Errorf("swapped host %q with luciProject %q", gerritHost, luciProject)
	}
	// TODO(crbug/824492): use auth.GetRPCTransport(ctx, auth.AsProject, ...)
	// directly after pssa migration is over. Currently, we need a special
	// error to detect whether pssa is configured or not.
	t, err := f.transport(gerritHost, luciProject)
	if err != nil {
		return nil, err
	}
	return gerrit.NewRESTClient(&http.Client{Transport: t}, gerritHost, true)
}

func (f *prodFactory) transport(gerritHost, luciProject string) (http.RoundTripper, error) {
	return luciauth.NewModifyingTransport(f.baseTransport, func(req *http.Request) error {
		tok, err := f.token(req.Context(), gerritHost, luciProject)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", tok.TokenType+" "+tok.AccessToken)
		return nil
	}), nil
}

func (f *prodFactory) token(ctx context.Context, gerritHost, luciProject string) (*oauth2.Token, error) {
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
	case token == nil:
		return nil, errors.Fmt("LUCI project: %q: %w", luciProject, errEmptyProjectToken)
	default:
		return &oauth2.Token{
			AccessToken: token.Token,
			TokenType:   "Bearer",
		}, nil
	}
}
