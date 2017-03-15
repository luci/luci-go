// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"encoding/base64"
	"fmt"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
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
	resp := &admin.InspectDelegationTokenResponse{}

	blob, err := base64.RawURLEncoding.DecodeString(req.Token)
	if err != nil {
		resp.InvalidityReason = fmt.Sprintf("not base64 - %s", err)
		return resp, nil
	}

	resp.Envelope = &messages.DelegationToken{}
	if err = proto.Unmarshal(blob, resp.Envelope); err != nil {
		resp.InvalidityReason = fmt.Sprintf("can't unmarshal the envelope - %s", err)
		return resp, nil
	}

	certs, err := r.Signer.Certificates(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Can't grab service certificates")
		return nil, grpc.Errorf(codes.Internal, "can't grab service certificates")
	}

	err = certs.CheckSignature(
		resp.Envelope.SigningKeyId, resp.Envelope.SerializedSubtoken, resp.Envelope.Pkcs1Sha256Sig)
	if errors.IsTransient(err) {
		logging.WithError(err).Errorf(c, "Transient error when checking signature")
		return nil, grpc.Errorf(codes.Internal, "transient error when checking signature")
	}
	resp.Signed = err == nil

	resp.Subtoken = &messages.Subtoken{}
	if err = proto.Unmarshal(resp.Envelope.SerializedSubtoken, resp.Subtoken); err != nil {
		resp.InvalidityReason = fmt.Sprintf("can't unmarshal token body - %s", err)
		return resp, nil
	}

	now := clock.Now(c).Unix()
	resp.NonExpired = (now >= resp.Subtoken.CreationTime) &&
		(now < resp.Subtoken.CreationTime+int64(resp.Subtoken.ValidityDuration))

	switch {
	case !resp.Signed:
		resp.InvalidityReason = "invalid signature"
	case !resp.NonExpired:
		resp.InvalidityReason = "expired or not active yet"
	default:
		resp.Valid = true
	}

	return resp, nil
}
