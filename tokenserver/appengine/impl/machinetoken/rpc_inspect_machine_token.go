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

package machinetoken

import (
	"context"
	"fmt"
	"math/big"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth/signing"

	tokenserver "go.chromium.org/luci/tokenserver/api"
	admin "go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/certchecker"
	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"
)

// InspectMachineTokenRPC implements Admin.InspectMachineToken API method.
//
// It assumes authorization has happened already.
type InspectMachineTokenRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is the default server signer that uses server's service account.
	Signer signing.Signer
}

// InspectMachineToken decodes a machine token and verifies it is valid.
func (r *InspectMachineTokenRPC) InspectMachineToken(c context.Context, req *admin.InspectMachineTokenRequest) (*admin.InspectMachineTokenResponse, error) {
	// Defaults.
	if req.TokenType == 0 {
		req.TokenType = tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN
	}

	// Only LUCI_MACHINE_TOKEN is supported currently.
	switch req.TokenType {
	case tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN:
		// supported
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported token type %s", req.TokenType)
	}

	// Deserialize the token, check its signature.
	inspection, err := InspectToken(c, r.Signer, req.Token)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%s", err)
	}
	resp := &admin.InspectMachineTokenResponse{
		Signed:           inspection.Signed,
		NonExpired:       inspection.NonExpired,
		InvalidityReason: inspection.InvalidityReason,
	}

	// Grab the signing key from the envelope, if managed to deserialize it.
	if env, _ := inspection.Envelope.(*tokenserver.MachineTokenEnvelope); env != nil {
		resp.SigningKeyId = env.KeyId
	}

	// If we could not deserialize the body, that's all checks we could do.
	body, _ := inspection.Body.(*tokenserver.MachineTokenBody)
	if body == nil {
		return resp, nil
	}
	resp.TokenType = &admin.InspectMachineTokenResponse_LuciMachineToken{
		LuciMachineToken: body,
	}

	addReason := func(r string) {
		if resp.InvalidityReason == "" {
			resp.InvalidityReason = r
		} else {
			resp.InvalidityReason += "; " + r
		}
	}

	// Check revocation status. Find CA name that signed the certificate used when
	// minting the token.
	caName, err := certconfig.GetCAByUniqueID(c, body.CaId)
	switch {
	case err != nil:
		return nil, status.Errorf(codes.Internal, "can't resolve ca_id to CA name - %s", err)
	case caName == "":
		addReason("no CA with given ID")
		return resp, nil
	}
	resp.CertCaName = caName

	// Grab CertChecker for this CA. It has CRL cached.
	certChecker, err := certchecker.GetCertChecker(c, caName)
	switch {
	case transient.Tag.In(err):
		return nil, status.Errorf(codes.Internal, "can't fetch CRL - %s", err)
	case err != nil:
		addReason(fmt.Sprintf("can't fetch CRL - %s", err))
		return resp, nil
	}

	// Check that certificate SN is not in the revocation list.
	sn := big.NewInt(0).SetBytes(body.CertSn)
	revoked, err := certChecker.CRL.IsRevokedSN(c, sn)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't check CRL - %s", err)
	}
	resp.NonRevoked = !revoked

	// Note: if Signed or NonExpired is false, InvalidityReason is already set.
	if resp.Signed && resp.NonExpired {
		if resp.NonRevoked {
			resp.Valid = true
		} else {
			addReason("corresponding cert was revoked")
		}
	}

	return resp, nil
}
