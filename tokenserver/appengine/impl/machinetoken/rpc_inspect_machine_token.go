// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package machinetoken

import (
	"fmt"
	"math/big"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/server/auth/signing"

	tokenserver "github.com/luci/luci-go/tokenserver/api"
	admin "github.com/luci/luci-go/tokenserver/api/admin/v1"

	"github.com/luci/luci-go/tokenserver/appengine/impl/certchecker"
	"github.com/luci/luci-go/tokenserver/appengine/impl/certconfig"
)

// InspectMachineTokenRPC implements Admin.InspectMachineToken API method.
//
// It assumes authorization has happened already.
type InspectMachineTokenRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is gaesigner.Signer.
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
		return nil, grpc.Errorf(codes.InvalidArgument, "unsupported token type %s", req.TokenType)
	}

	// Deserialize the token, don't do any validity checks yet.
	envelope, body, err := Parse(req.Token)
	switch {
	case envelope == nil:
		return &admin.InspectMachineTokenResponse{
			InvalidityReason: fmt.Sprintf("bad envelope format - %s", err),
		}, nil
	case body == nil:
		return &admin.InspectMachineTokenResponse{
			SigningKeyId:     envelope.KeyId,
			InvalidityReason: fmt.Sprintf("bad body format - %s", err),
		}, nil
	case err != nil:
		return &admin.InspectMachineTokenResponse{
			InvalidityReason: fmt.Sprintf("parsing error - %s", err),
		}, nil
	}

	resp := &admin.InspectMachineTokenResponse{
		InvalidityReason: "unknown", // will be replaced below
		SigningKeyId:     envelope.KeyId,
		TokenType: &admin.InspectMachineTokenResponse_LuciMachineToken{
			LuciMachineToken: body,
		},
	}

	// Check that the token was signed by our private key.
	certs, err := r.Signer.Certificates(c)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't fetch service certificates - %s", err)
	}
	err = CheckSignature(envelope, certs)
	if err != nil {
		resp.InvalidityReason = fmt.Sprintf("can't validate signature - %s", err)
		return resp, nil
	}
	resp.Signed = true

	// Check the expiration time. Allow 10 sec clock drift.
	resp.NonExpired = !IsExpired(body, clock.Now(c))

	// Check revocation status. Find CA name that signed the certificate used when
	// minting the token.
	caName, err := certconfig.GetCAByUniqueID(c, body.CaId)
	switch {
	case err != nil:
		return nil, grpc.Errorf(codes.Internal, "can't resolve ca_id to CA name - %s", err)
	case caName == "":
		resp.InvalidityReason = "no CA with given ID"
		return resp, nil
	}
	resp.CertCaName = caName

	// Grab CertChecker for this CA. It has CRL cached.
	certChecker, err := certchecker.GetCertChecker(c, caName)
	switch {
	case errors.IsTransient(err):
		return nil, grpc.Errorf(codes.Internal, "can't fetch CRL - %s", err)
	case err != nil:
		resp.InvalidityReason = fmt.Sprintf("can't fetch CRL - %s", err)
		return resp, nil
	}

	// Check that certificate SN is not in the revocation list.
	sn := big.NewInt(0).SetUint64(body.CertSn)
	revoked, err := certChecker.CRL.IsRevokedSN(c, sn)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't check CRL - %s", err)
	}
	resp.NonRevoked = !revoked

	// Pick invalidity reason (if any).
	switch {
	case !resp.NonExpired:
		resp.InvalidityReason = "expired"
	case !resp.NonRevoked:
		resp.InvalidityReason = "revoked"
	default:
		resp.Valid = true
		resp.InvalidityReason = ""
	}

	return resp, nil
}
