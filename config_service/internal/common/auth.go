// Copyright 2023 The LUCI Authors.
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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	credentials "cloud.google.com/go/iam/credentials/apiv1"
	"cloud.google.com/go/iam/credentials/apiv1/credentialspb"
	"google.golang.org/api/option"

	luciauth "go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/iam"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/srvhttp"
)

// GetSelfSignedJWTTransport returns a transport that add self signed jwt token
// as authorization header.
func GetSelfSignedJWTTransport(ctx context.Context, aud string) (http.RoundTripper, error) {
	signer := auth.GetSigner(ctx)
	info, err := signer.ServiceInfo(ctx)
	switch {
	case err != nil:
		return nil, fmt.Errorf("failed to get service account for the service: %w", err)
	case info.ServiceAccountName == "":
		return nil, errors.New("the current service account is empty")
	}
	serviceAccount := info.ServiceAccountName
	return luciauth.NewModifyingTransport(srvhttp.DefaultTransport(ctx), func(req *http.Request) error {
		ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes(scopes.CloudScopeSet()...))
		if err != nil {
			return fmt.Errorf("failed to get OAuth2 token source: %w", err)
		}
		cc, err := credentials.NewIamCredentialsClient(ctx, option.WithTokenSource(ts))
		if err != nil {
			return fmt.Errorf("failed to create IAM Credentials client: %w", err)
		}
		defer func() { _ = cc.Close() }()

		now := clock.Now(ctx).UTC()
		cs := &iam.ClaimSet{
			Iss:   serviceAccount,
			Scope: strings.Join(scopes.CloudScopeSet(), " "),
			Aud:   aud,
			Exp:   now.Add(2 * time.Minute).Unix(),
			Iat:   now.Unix(),
		}
		payload, err := json.Marshal(cs)
		if err != nil {
			return fmt.Errorf("failed to marshall claim set to JSON: %w", err)
		}
		res, err := cc.SignJwt(ctx, &credentialspb.SignJwtRequest{
			Name:    fmt.Sprintf("projects/-/serviceAccounts/%s", serviceAccount),
			Payload: string(payload),
		})
		if err != nil {
			return fmt.Errorf("failed to signJwt: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+res.SignedJwt)
		return nil
	}), nil
}
