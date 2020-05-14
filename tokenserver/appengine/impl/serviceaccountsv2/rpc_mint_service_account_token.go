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

package serviceaccountsv2

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/tokenserver/api/minter/v1"
)

// MintServiceAccountTokenRPC implements the corresponding method.
type MintServiceAccountTokenRPC struct {
}

// MintOAuthTokenViaGrant produces new OAuth token given a grant.
func (r *MintServiceAccountTokenRPC) MintServiceAccountToken(c context.Context, req *minter.MintServiceAccountTokenRequest) (*minter.MintServiceAccountTokenResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "not implemented yet")
}
