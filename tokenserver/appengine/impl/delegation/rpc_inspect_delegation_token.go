// Copyright 2016 The LUCI Authors.
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

package delegation

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/server/auth/signing"

	admin "go.chromium.org/luci/tokenserver/api/admin/v1"
)

// InspectDelegationTokenRPC implements Admin.InspectDelegationToken RPC method.
//
// It assumes authorization has happened already.
type InspectDelegationTokenRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is gaesigner.Signer.
	Signer signing.Signer
}

func (r *InspectDelegationTokenRPC) InspectDelegationToken(c context.Context, req *admin.InspectDelegationTokenRequest) (*admin.InspectDelegationTokenResponse, error) {
	inspection, err := InspectToken(c, r.Signer, req.Token)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, err.Error())
	}
	resp := &admin.InspectDelegationTokenResponse{
		Valid:            inspection.Signed && inspection.NonExpired,
		Signed:           inspection.Signed,
		NonExpired:       inspection.NonExpired,
		InvalidityReason: inspection.InvalidityReason,
	}
	resp.Envelope, _ = inspection.Envelope.(*messages.DelegationToken)
	resp.Subtoken, _ = inspection.Body.(*messages.Subtoken)
	return resp, nil
}
