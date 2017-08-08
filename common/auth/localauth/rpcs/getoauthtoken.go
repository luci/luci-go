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

package rpcs

import (
	"go.chromium.org/luci/common/errors"
)

// GetOAuthTokenRequest is parameters for GetOAuthToken RPC call.
type GetOAuthTokenRequest struct {
	Scopes    []string `json:"scopes"`
	Secret    []byte   `json:"secret"`
	AccountID string   `json:"account_id"`
}

// Validate checks that the request is structurally valid.
func (r *GetOAuthTokenRequest) Validate() error {
	switch {
	case len(r.Scopes) == 0:
		return errors.New(`field "scopes" is required`)
	case len(r.Secret) == 0:
		return errors.New(`field "secret" is required`)
	}
	// TODO(vadimsh): Uncomment when all old auth server are turned off.
	//case r.AccountID == "":
	//	return errors.New(`field "account_id" is required`)
	//}
	return nil
}

// GetOAuthTokenResponse is returned by GetOAuthToken RPC call.
type GetOAuthTokenResponse struct {
	BaseResponse

	AccessToken string `json:"access_token"`
	Expiry      int64  `json:"expiry"`
}
