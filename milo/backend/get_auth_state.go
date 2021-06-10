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

package backend

import (
	"context"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"google.golang.org/protobuf/types/known/timestamppb"

	milopb "go.chromium.org/luci/milo/api/service/v1"
)

// GetAuthState implements milopb.MiloInternal service
func (s *MiloInternalService) GetAuthState(ctx context.Context, req *milopb.GetAuthStateRequest) (res *milopb.GetAuthStateResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()

	user := auth.CurrentUser(ctx)
	if user.Identity == identity.AnonymousIdentity {
		return &milopb.GetAuthStateResponse{
			Identity: string(user.Identity),
		}, nil
	}

	token, err := auth.GetState(ctx).Session().AccessToken(ctx)
	if err != nil {
		return nil, err
	}

	return &milopb.GetAuthStateResponse{
		Identity:          string(user.Identity),
		Email:             user.Email,
		Picture:           user.Picture,
		AccessToken:       token.AccessToken,
		AccessTokenExpiry: timestamppb.New(token.Expiry),
	}, nil
}
