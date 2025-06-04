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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
)

// signerFactory produces a signer on demand.
type signerFactory func(context.Context) (*signer, error)

// signer can RSA-sign blobs the way Google Storage likes it.
//
// Mocked in tests.
type signer struct {
	Email     string
	SignBytes func(context.Context, []byte) (key string, sig []byte, err error)
}

// defaultSigner uses the default server account for signing.
func defaultSigner(ctx context.Context) (*signer, error) {
	s := auth.GetSigner(ctx)
	if s == nil {
		return nil, errors.New("a default signer is not available")
	}
	info, err := s.ServiceInfo(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to grab the signer info: %w", err)
	}
	return &signer{
		Email:     info.ServiceAccountName,
		SignBytes: s.SignBytes,
	}, nil
}
