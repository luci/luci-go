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

package eval

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"cloud.google.com/go/bigquery"
	"golang.org/x/time/rate"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

const day = 24 * time.Hour

// Init initializes e.
func (r *evalRun) Init(ctx context.Context) error {
	if r.Concurrency <= 0 {
		r.Concurrency = defaultConcurrency
	}
	if r.GerritQPSLimit <= 0 {
		r.GerritQPSLimit = defaultGerritQPSLimit
	}

	// Ensure we have a cache dir.
	var err error
	if r.CacheDir == "" {
		if r.CacheDir, err = defaultCacheDir(); err != nil {
			return err
		}
	}

	// Init auth.
	authOpts := chromeinfra.DefaultAuthOptions()
	authOpts.Scopes = []string{auth.OAuthScopeEmail, bigquery.Scope, gerrit.OAuthScope}
	r.auth = auth.NewAuthenticator(ctx, auth.InteractiveLogin, authOpts)

	if r.gerrit, err = r.newGerritClient(r.auth); err != nil {
		return errors.Annotate(err, "failed to init Gerrit client").Err()
	}

	return nil
}

// newGerritClient creates a new gitiles client. Does not mutate r.
func (r *evalRun) newGerritClient(authenticator *auth.Authenticator) (*gerritClient, error) {
	transport, err := authenticator.Transport()
	if err != nil {
		return nil, err
	}

	// Note: Gerrit quota is shared across all Gerrit hosts, so we should not use
	// a rate limiter per host.
	limit := rate.Limit(float64(r.GerritQPSLimit))
	if limit <= 0 {
		limit = 10
	}

	httpClient := &http.Client{Transport: transport}

	return &gerritClient{
		listFilesRPC: func(ctx context.Context, host string, req *gerritpb.ListFilesRequest) (*gerritpb.ListFilesResponse, error) {
			client, err := gerrit.NewRESTClient(httpClient, host, true)
			if err != nil {
				return nil, errors.Annotate(err, "failed to create a Gerrit client").Err()
			}
			return client.ListFiles(ctx, req)
		},
		fileListCache: cache{
			dir:       filepath.Join(r.CacheDir, "gerrit-changed-files"),
			memory:    lru.New(1024),
			valueType: reflect.TypeOf(changedFiles{}),
		},
		limiter: rate.NewLimiter(limit, 1),
	}, nil
}

func defaultCacheDir() (string, error) {
	ucd, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(ucd, "chrome-rts"), nil
}
