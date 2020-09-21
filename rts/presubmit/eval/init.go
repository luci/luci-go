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
	"time"

	"cloud.google.com/go/bigquery"
	"golang.org/x/time/rate"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

const day = 24 * time.Hour

// Init initializes e.
func (e *evalRun) Init(ctx context.Context) error {
	switch {
	case e.WindowsDays <= 0:
		return errors.New("-window must be positive")
	case e.Concurrency < 0:
		return errors.New("-j must be positive")
	}

	e.endTime = time.Now().UTC().Add(-day).Truncate(day)
	e.startTime = e.endTime.Add(-day * time.Duration(e.WindowsDays))

	// Init auth.
	authOpts := chromeinfra.DefaultAuthOptions()
	authOpts.Scopes = []string{auth.OAuthScopeEmail, bigquery.Scope, gerrit.OAuthScope}
	e.auth = auth.NewAuthenticator(ctx, auth.InteractiveLogin, authOpts)

	var err error
	if e.gerrit, err = e.newGerritClient(e.auth); err != nil {
		return errors.Annotate(err, "failed to init Gerrit client").Err()
	}

	return nil
}

// newGerritClient creates a new gitiles client. Does not mutate r.
func (e *evalRun) newGerritClient(authenticator *auth.Authenticator) (*gerritClient, error) {
	transport, err := authenticator.Transport()
	if err != nil {
		return nil, err
	}

	// Note: "gitiles view" quota is per Borg cell (not host),
	// so we should not use a rate limiter per host.
	limit := rate.Limit(float64(e.GerritQPSLimit))
	if limit <= 0 {
		limit = 10
	}

	return &gerritClient{
		httpClient: &http.Client{Transport: transport},
		limiter:    rate.NewLimiter(limit, 1),
	}, nil
}
