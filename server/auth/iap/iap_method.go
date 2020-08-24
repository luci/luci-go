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

package iap

import (
	"errors"
	"fmt"
	"net/http"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"golang.org/x/net/context"
	"google.golang.org/api/idtoken"
)

var (
	ErrMissingJWTAssertionHeader = errors.New("No X-Goog-Iap-Jwt-Assertion header present")
)

const (
	IAPJWTAssertionHeader = "X-Goog-Iap-Jwt-Assertion"
)

type IDTokenValidator func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error)

/**
 * IAPAuthMethod implements auth.Method for use with GCP's Identity Aware Proxy.
 * For production use, one should use the cloud.google.com/go/compute/metadata to get
 * the NumericProjectID and AppID properties on process startup.
 */
type IAPAuthMethod struct {
	// NumericProjectID is a string-formatted integer ID that identifies the GCP project.
	NumericProjectID string
	// AppID is the appengine project id.
	AppID string
	// Validator is called by Authenticate to validate tokens. If nil, Authenticate will
	// use idtoken.Validate by default.
	Validator IDTokenValidator
}

func (a *IAPAuthMethod) aud() string {
	return fmt.Sprintf("/projects/%s/apps/%s", a.NumericProjectID, a.AppID)
}

// Authenticate returns a User if authentication is successful, otherwise it returns
// an error.
func (a *IAPAuthMethod) Authenticate(ctx context.Context, r *http.Request) (*auth.User, error) {
	iapJwt, ok := r.Header[IAPJWTAssertionHeader]
	if !ok {
		logging.Errorf(ctx, "iap: missing assertion header")
		return nil, ErrMissingJWTAssertionHeader
	}

	validateFunc := a.Validator
	if validateFunc == nil {
		validateFunc = idtoken.Validate
	}

	if len(iapJwt) != 1 {
		return nil, fmt.Errorf("iap: expected 1 value for assertion header, but found %d", len(iapJwt))
	}

	jwtPayload, err := validateFunc(ctx, iapJwt[0], a.aud())
	if err != nil {
		logging.Errorf(ctx, "couldn't validate jwt payload for aud: %q", err)
		return nil, err
	}

	email, ok := jwtPayload.Claims["email"]
	if !ok {
		return nil, fmt.Errorf("iap: no email claim")
	}

	emailStr := email.(string)
	ret := &auth.User{
		Identity: identity.Identity("user:" + emailStr),
		Email:    emailStr,
	}

	return ret, nil
}
