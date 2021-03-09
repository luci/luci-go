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

package rpcs

import (
	"go.chromium.org/luci/common/errors"
)

// GetIDTokenRequest is parameters for GetIDToken RPC call.
type GetIDTokenRequest struct {
	BaseRequest

	Audience string `json:"audience"`
}

// Validate checks that the request is structurally valid.
func (r *GetIDTokenRequest) Validate() error {
	if r.Audience == "" {
		return errors.New(`field "audience" is required`)
	}
	return r.BaseRequest.Validate()
}

// GetIDTokenResponse is returned by GetIDToken RPC call.
type GetIDTokenResponse struct {
	BaseResponse

	IDToken string `json:"id_token"`
	Expiry  int64  `json:"expiry"`
}
