// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/server/auth/delegation/messages"
	"github.com/luci/luci-go/server/auth/signing"

	admin "github.com/luci/luci-go/tokenserver/api/admin/v1"
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
