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
	"io"
	"io/ioutil"
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

// HitTokenEndpoint sends a request to the token provider's endpoint.
//
// Returns the produced tokens and their expiry time.
func HitTokenEndpoint(ctx context.Context, doc *openid.DiscoveryDoc, params map[string]string) (*sessionpb.Private, time.Time, error) {
	form := make(url.Values, len(params))
	for k, v := range params {
		form.Set(k, v)
	}
	body := strings.NewReader(form.Encode())

	tr, err := auth.GetRPCTransport(ctx, auth.NoAuth)
	if err != nil {
		return nil, time.Time{}, errors.Annotate(err, "failed to get the transport").Err()
	}
	client := &http.Client{Transport: tr}

	resp, err := client.Post(doc.TokenEndpoint, "application/x-www-form-urlencoded", body)
	if resp != nil {
		defer func() {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}()
	}
	if err != nil {
		return nil, time.Time{}, errors.Annotate(err, "failed to call the token endpoint").Tag(transient.Tag).Err()
	}
	blob, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, time.Time{}, errors.Annotate(err, "failed to read the token endpoint's response").Tag(transient.Tag).Err()
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		err := errors.Reason("got HTTP %d from the token endpoint with body %q", resp.StatusCode, blob)
		if resp.StatusCode >= 500 {
			err = err.Tag(transient.Tag)
		}
		return nil, time.Time{}, err.Err()
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
