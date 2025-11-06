// Copyright 2021 The LUCI Authors.
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

package auth

import (
	"context"
	"net/http"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/gcloud/iam"
	"go.chromium.org/luci/common/retry/transient"
)

var defaultActorTokensProviderImpl = defaultActorTokensProvider{}

// defaultActorTokensProvider implements ActorTokensProvider via IAM Credentials
// REST API.
//
// It is currently used from GAEv1 services. Can be removed once all services
// are GAEv2 (i.e. use luci/server).
type defaultActorTokensProvider struct{}

// GenerateAccessToken generates an access token for the given account.
func (defaultActorTokensProvider) GenerateAccessToken(ctx context.Context, serviceAccount string, scopes, delegates []string) (tok *oauth2.Token, err error) {
	err = withCredentialsClient(ctx, func(client *iam.CredentialsClient) (err error) {
		tok, err = client.GenerateAccessToken(ctx, serviceAccount, scopes, delegatesList(delegates), 0)
		return
	})
	return
}

// GenerateIDToken generates an ID token for the given account.
func (defaultActorTokensProvider) GenerateIDToken(ctx context.Context, serviceAccount, audience string, delegates []string) (tok string, err error) {
	err = withCredentialsClient(ctx, func(client *iam.CredentialsClient) (err error) {
		tok, err = client.GenerateIDToken(ctx, serviceAccount, audience, true, delegatesList(delegates))
		return
	})
	return
}

func withCredentialsClient(ctx context.Context, cb func(client *iam.CredentialsClient) error) error {
	// Need an authenticating transport to talk to IAM.
	asSelf, err := GetRPCTransport(ctx, AsSelf, WithScopes(scopes.CloudScopeSet()...))
	if err != nil {
		return err
	}
	client := &iam.CredentialsClient{Client: &http.Client{Transport: asSelf}}

	// CredentialsClient returns googleapi.Error on HTTP-level responses.
	// Recognize fatal HTTP errors. Everything else (stuff like connection
	// timeouts, deadlines, etc) are transient errors.
	if err = cb(client); err != nil {
		if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code < 500 {
			return err
		}
		return transient.Tag.Apply(err)
	}
	return nil
}

// delegatesList prepends `projects/-/serviceAccounts/` to emails.
func delegatesList(emails []string) []string {
	if len(emails) == 0 {
		return nil
	}
	out := make([]string, len(emails))
	for i, email := range emails {
		out[i] = "projects/-/serviceAccounts/" + email
	}
	return out
}
