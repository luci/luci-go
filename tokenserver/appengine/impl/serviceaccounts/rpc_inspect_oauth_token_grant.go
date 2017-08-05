// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package serviceaccounts

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/server/auth/signing"

	"github.com/luci/luci-go/tokenserver/api"
	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

// InspectOAuthTokenGrantRPC implements admin.InspectOAuthTokenGrant method.
type InspectOAuthTokenGrantRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is gaesigner.Signer.
	Signer signing.Signer
}

// InspectOAuthTokenGrant decodes the given OAuth token grant.
func (r *InspectOAuthTokenGrantRPC) InspectOAuthTokenGrant(c context.Context, req *admin.InspectOAuthTokenGrantRequest) (*admin.InspectOAuthTokenGrantResponse, error) {
	inspection, err := InspectGrant(c, r.Signer, req.Token)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, err.Error())
	}
	resp := &admin.InspectOAuthTokenGrantResponse{
		Valid:            inspection.Signed && inspection.NonExpired,
		Signed:           inspection.Signed,
		NonExpired:       inspection.NonExpired,
		InvalidityReason: inspection.InvalidityReason,
	}
	if env, _ := inspection.Envelope.(*tokenserver.OAuthTokenGrantEnvelope); env != nil {
		resp.SigningKeyId = env.KeyId
	}
	resp.TokenBody, _ = inspection.Body.(*tokenserver.OAuthTokenGrantBody)
	return resp, nil
}
