// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package googleoauth

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
	"google.golang.org/api/googleapi"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	// TokeninfoEndpoint is Google's token info endpoint.
	TokeninfoEndpoint = "https://www.googleapis.com/oauth2/v3/tokeninfo"
)

// ErrBadToken is returned by GetTokenInfo if the passed token is invalid.
var ErrBadToken = errors.New("bad token")

// TokenInfoParams are parameters for GetTokenInfo call.
type TokenInfoParams struct {
	AccessToken string // an access token to check
	IDToken     string // an ID token to check (overrides AccessToken)

	Client   *http.Client // non-authenticating client to use for the call
	Endpoint string       // an endpoint to use instead of the default one
}

// TokenInfo is information about an access or ID tokens.
//
// Of primary importance are 'email', 'email_verified', 'scope' and 'aud'
// fields. If the caller using token info endpoint to validate tokens, it MUST
// check correctness of these fields.
type TokenInfo struct {
	Azp           string `json:"azp"`
	Aud           string `json:"aud"`
	Sub           string `json:"sub"`
	Scope         string `json:"scope"`
	Exp           int64  `json:"exp,string"`
	ExpiresIn     int64  `json:"expires_in,string"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified,string"`
	AccessType    string `json:"access_type"`
}

// GetTokenInfo queries token info endpoint and returns information about
// the token if it is recognized.
//
// See https://developers.google.com/identity/sign-in/android/backend-auth#calling-the-tokeninfo-endpoint.
//
// On invalid token (as indicated by 4** HTTP response) returns ErrBadToken. On
// other HTTP-level errors (e.g HTTP 500) returns transient-wrapped
// *googleapi.Error. On network-level errors returns them in a transient
// wrapper.
func GetTokenInfo(c context.Context, params TokenInfoParams) (*TokenInfo, error) {
	if params.Client == nil {
		params.Client = http.DefaultClient
	}
	if params.Endpoint == "" {
		params.Endpoint = TokeninfoEndpoint
	}

	// Note: we must not log full URL of this call, it contains sensitive info.
	logging.Debugf(c, "POST %s", params.Endpoint)
	v := url.Values{}
	if params.IDToken != "" {
		v.Add("id_token", params.IDToken)
	} else {
		v.Add("access_token", params.AccessToken)
	}
	resp, err := ctxhttp.Get(c, params.Client, params.Endpoint+"?"+v.Encode())
	if err != nil {
		logging.WithError(err).Errorf(c, "POST %s failed", params.Endpoint)
		return nil, transient.Tag.Apply(err)
	}
	defer googleapi.CloseBody(resp)
	if err := googleapi.CheckResponse(resp); err != nil {
		logging.WithError(err).Errorf(c, "POST %s failed", params.Endpoint)
		if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code < 500 {
			return nil, ErrBadToken
		}
		return nil, transient.Tag.Apply(err)
	}

	info := &TokenInfo{}
	if err := json.NewDecoder(resp.Body).Decode(info); err != nil {
		// This should never happen. If it does, the token endpoint has gone mad,
		// and maybe it will recover soon. So mark the error as transient.
		logging.WithError(err).Errorf(c, "Bad token info endpoint response")
		return nil, transient.Tag.Apply(err)
	}

	return info, nil
}
