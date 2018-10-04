// Copyright 2017 The LUCI Authors.
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

package cas

import (
	"context"
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/iam"
	"go.chromium.org/luci/server/auth"
)

// signerFactory produces a signer on demand.
type signerFactory func(context.Context) (*signer, error)

// signer can RSA-sign blobs the way Google Storage likes it.
type signer struct {
	Email     string
	SignBytes func(context.Context, []byte) (key string, sig []byte, err error)
}

// defaultSigner uses the default app account for signing.
func defaultSigner(c context.Context) (*signer, error) {
	s := auth.GetSigner(c)
	if s == nil {
		return nil, errors.Reason("a default signer is not available").Err()
	}
	info, err := s.ServiceInfo(c)
	if err != nil {
		return nil, errors.Annotate(err, "failed to grab the signer info").Err()
	}
	return &signer{
		Email:     info.ServiceAccountName,
		SignBytes: s.SignBytes,
	}, nil
}

// iamSigner uses SignBytes IAM API for signing.
func iamSigner(c context.Context, actAs string) (*signer, error) {
	t, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(iam.OAuthScope))
	if err != nil {
		return nil, errors.Annotate(err, "failed to grab RPC transport").Err()
	}
	s := &iam.Signer{
		Client:         &iam.Client{Client: &http.Client{Transport: t}},
		ServiceAccount: actAs,
	}
	return &signer{
		Email:     actAs,
		SignBytes: s.SignBytes,
	}, nil
}
