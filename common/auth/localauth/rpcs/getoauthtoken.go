// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package rpcs

import (
	"github.com/luci/luci-go/common/errors"
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
