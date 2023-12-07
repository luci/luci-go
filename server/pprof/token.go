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

package pprof

import (
	"context"
	"time"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tokens"
)

// pprofToken described how to generate tokens.
var pprofToken = tokens.TokenKind{
	Algo:       tokens.TokenAlgoHmacSHA256,
	Expiration: 12 * time.Hour,
	SecretKey:  "pprof_token",
	Version:    1,
}

// generateToken generates new pprof token.
//
// The token is URL safe base64 encoded string. Possession of such token allows
// to call pprof endpoints.
//
// The token expires after 12 hours.
func generateToken(c context.Context) (string, error) {
	logging.Warningf(c, "%q is generating pprof token", auth.CurrentIdentity(c))
	return pprofToken.Generate(c, nil, nil, 0)
}

// checkToken succeeds if the given pprof token is valid and non-expired.
func checkToken(c context.Context, tok string) error {
	_, err := pprofToken.Validate(c, tok, nil)
	return err
}
