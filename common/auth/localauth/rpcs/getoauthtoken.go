// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package rpcs

import (
	"fmt"
)

// GetOAuthTokenRequest is parameters for GetOAuthToken RPC call.
type GetOAuthTokenRequest struct {
	Scopes []string `json:"scopes"`
	Secret []byte   `json:"secret"`
}

// Validate checks that scopes and secret are set.
func (r *GetOAuthTokenRequest) Validate() error {
	switch {
	case len(r.Scopes) == 0:
		return fmt.Errorf(`Field "scopes" is required.`)
	case len(r.Secret) == 0:
		return fmt.Errorf(`Field "secret" is required.`)
	}
	return nil
}

// GetOAuthTokenResponse is returned by GetOAuthToken RPC call.
type GetOAuthTokenResponse struct {
	BaseResponse

	AccessToken string `json:"access_token"`
	Expiry      int64  `json:"expiry"`
}
