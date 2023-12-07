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

package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/encryptedcookies/session/sessionpb"
)

// EndpointError is returned on recognized error responses.
//
// If the provider replies with some gibberish, some generic error will be
// returned instead.
type EndpointError struct {
	Code        string `json:"error"`             // e.g. invalid_grant
	Description string `json:"error_description"` // human readable text
}

// Error makes EndpointError implement `error` interface.
func (ee *EndpointError) Error() string {
	return fmt.Sprintf("%s: %s", ee.Code, ee.Description)
}

// HitTokenEndpoint sends a request to the OpenID provider's token endpoint.
//
// Returns the produced tokens and their expiry time. Tags errors as transient
// if necessary.
func HitTokenEndpoint(ctx context.Context, doc *openid.DiscoveryDoc, params map[string]string) (*sessionpb.Private, time.Time, error) {
	blob, err := hitEndpoint(ctx, doc.TokenEndpoint, params)
	if err != nil {
		return nil, time.Time{}, err
	}

	var tokens struct {
		RefreshToken string `json:"refresh_token"`
		AccessToken  string `json:"access_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int64  `json:"expires_in"` // sec
	}
	if err := json.Unmarshal(blob, &tokens); err != nil {
		return nil, time.Time{}, errors.Annotate(err, "failed to unmarshal ID providers response %q", string(blob)).Err()
	}

	exp := clock.Now(ctx).Add(time.Duration(tokens.ExpiresIn) * time.Second)

	return &sessionpb.Private{
		RefreshToken: tokens.RefreshToken,
		AccessToken:  tokens.AccessToken,
		IdToken:      tokens.IDToken,
	}, exp, nil
}

// HitRevocationEndpoint sends a request to the OpenID provider's revocation
// endpoint.
//
// Returns nil if the token was successfully revoked or it is already invalid.
func HitRevocationEndpoint(ctx context.Context, doc *openid.DiscoveryDoc, params map[string]string) error {
	_, err := hitEndpoint(ctx, doc.RevocationEndpoint, params)
	if apiErr, ok := err.(*EndpointError); ok && apiErr.Code == "invalid_token" {
		return nil // was already revoked, it is OK
	}
	return err
}

// hitEndpoint sends a request to an OpenID provider's token endpoint.
//
// Returns:
//
//	(body, nil) on a HTTP 200 reply.
//	(nil, *EndpointError) on recognized fatal errors.
//	(nil, non-transient error) on unrecognized fatal errors.
//	(nil, transient error) on transient errors.
func hitEndpoint(ctx context.Context, endpoint string, params map[string]string) ([]byte, error) {
	tr, err := auth.GetRPCTransport(ctx, auth.NoAuth)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get the transport").Err()
	}
	client := &http.Client{Transport: tr}

	form := make(url.Values, len(params))
	for k, v := range params {
		form.Set(k, v)
	}
	body := strings.NewReader(form.Encode())

	req, err := http.NewRequest("POST", endpoint, body)
	if err != nil {
		return nil, errors.Annotate(err, "bad OpenID provider request request").Err()
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req.WithContext(ctx))
	if resp != nil {
		defer func() {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()
	}
	if err != nil {
		return nil, errors.Annotate(err, "failed to call %q", endpoint).Tag(transient.Tag).Err()
	}
	blob, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read the response from %q", endpoint).Tag(transient.Tag).Err()
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		var apiErr EndpointError
		if json.Unmarshal(blob, &apiErr) == nil {
			err = &apiErr
		} else {
			err = errors.Reason("got HTTP %d from %q with body %q", resp.StatusCode, endpoint, blob).Err()
		}
		if resp.StatusCode >= 500 {
			err = transient.Tag.Apply(err)
		}
		return nil, err
	}

	return blob, nil
}
