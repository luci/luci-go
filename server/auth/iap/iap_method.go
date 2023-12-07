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

// Package iap implements auth.Method for GCP's Identity Aware Proxy.
// It does payload verification according to the guide for using signed
// headers: https://cloud.google.com/iap/docs/signed-headers-howto#verifying_the_jwt_payload
package iap

import (
	"context"
	"fmt"

	"google.golang.org/api/idtoken"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth"
)

const (
	iapJWTAssertionHeader = "X-Goog-Iap-Jwt-Assertion"
)

// AudForGAE returns an audience string for the GAE application as it will be formatted
// by IAP in the aseertion headers. This is a convenience method.
// For production use, one should use the cloud.google.com/go/compute/metadata to get
// the NumericProjectID and AppID properties on process startup.
func AudForGAE(numericProjectID, appID string) string {
	return fmt.Sprintf("/projects/%s/apps/%s", numericProjectID, appID)
}

// AudForGlobalBackendService returns an audience string for a GCE or GKE application as
// it will be formatted by IAP in the aseertion headers. This is a convenience method.
func AudForGlobalBackendService(projectNumber, backendServiceID string) string {
	return fmt.Sprintf("/projects/%s/global/backendServices/%s", projectNumber, backendServiceID)
}

type idTokenValidator func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error)

// IAPAuthMethod implements auth.Method for use with GCP's Identity Aware Proxy.
type IAPAuthMethod struct {
	// Aud is the audience string as it should appear in JWTs intended for
	// validation by your service.
	Aud string

	// validator field is only intended for use by unit tests.
	validator idTokenValidator
}

// Authenticate returns nil if no IAP assertion header is present, a User if authentication
// is successful, or an error if unable to validate and identify a user from the assertion header.
func (a *IAPAuthMethod) Authenticate(ctx context.Context, r auth.RequestMetadata) (*auth.User, auth.Session, error) {
	iapJwt := r.Header(iapJWTAssertionHeader)
	if iapJwt == "" {
		logging.Errorf(ctx, "iap: missing assertion header")
		return nil, nil, nil
	}

	validateFunc := a.validator
	if validateFunc == nil {
		validateFunc = idtoken.Validate
	}

	jwtPayload, err := validateFunc(ctx, iapJwt, a.Aud)
	if err != nil {
		return nil, nil, errors.Annotate(err, "couldn't validate jwt payload").Err()
	}

	email, ok := jwtPayload.Claims["email"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("iap: no email claim")
	}

	return &auth.User{
		Identity: identity.Identity("user:" + email),
		Email:    email,
	}, nil, nil
}
