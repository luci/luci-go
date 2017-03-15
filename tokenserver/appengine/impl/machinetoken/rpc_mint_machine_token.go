// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package machinetoken

import (
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/server/auth/signing"

	tokenserver "github.com/luci/luci-go/tokenserver/api"
	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/api/minter/v1"

	"github.com/luci/luci-go/tokenserver/appengine/impl/certchecker"
	"github.com/luci/luci-go/tokenserver/appengine/impl/certconfig"
)

// MintMachineTokenRPC implements TokenMinter.MintMachineToken RPC method.
type MintMachineTokenRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is gaesigner.Signer.
	Signer signing.Signer

	// CheckCertificate is mocked in tests.
	//
	// In prod it is certchecker.CheckCertificate.
	CheckCertificate func(c context.Context, cert *x509.Certificate) (*certconfig.CA, error)
}

// serviceVersion returns a string that identifier the app and the version.
//
// It is put in server responses.
//
// This function almost never returns errors. It can return an error only when
// called for the first time during the process lifetime. It gets cached after
// first successful return.
func (r *MintMachineTokenRPC) serviceVersion(c context.Context) (string, error) {
	inf, err := r.Signer.ServiceInfo(c) // cached
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", inf.AppID, inf.AppVersion), nil
}

// MintMachineToken generates a new token for an authenticated machine.
func (r *MintMachineTokenRPC) MintMachineToken(c context.Context, req *minter.MintMachineTokenRequest) (*minter.MintMachineTokenResponse, error) {
	// Parse serialized portion of the request and do minimal validation before
	// checking the signature to reject obviously bad requests.
	if len(req.SerializedTokenRequest) == 0 {
		return nil, grpc.Errorf(codes.InvalidArgument, "empty request")
	}
	tokenReq := minter.MachineTokenRequest{}
	if err := proto.Unmarshal(req.SerializedTokenRequest, &tokenReq); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "failed to unmarshal TokenRequest - %s", err)
	}

	switch tokenReq.TokenType {
	case tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN:
		// supported
	default:
		return r.mintingErrorResponse(
			c, minter.ErrorCode_UNSUPPORTED_TOKEN_TYPE,
			"token_type %s is not supported", tokenReq.TokenType)
	}

	// Timestamp is required.
	issuedAt := google.TimeFromProto(tokenReq.IssuedAt)
	if issuedAt.IsZero() {
		return r.mintingErrorResponse(c, minter.ErrorCode_BAD_TIMESTAMP, "issued_at is required")
	}

	// It should be within acceptable range.
	now := clock.Now(c)
	notBefore := now.Add(-10 * time.Minute)
	notAfter := now.Add(10 * time.Minute)
	if issuedAt.Before(notBefore) || issuedAt.After(notAfter) {
		return r.mintingErrorResponse(
			c, minter.ErrorCode_BAD_TIMESTAMP,
			"issued_at timestamp is not within acceptable range, check your clock")
	}

	// The certificate must be valid.
	cert, err := x509.ParseCertificate(tokenReq.Certificate)
	if err != nil {
		return r.mintingErrorResponse(
			c, minter.ErrorCode_BAD_CERTIFICATE_FORMAT,
			"failed to parse the certificate (expecting x509 cert DER)")
	}

	// Check the signature before proceeding. Use switch when picking an algo
	// as a reminder to add a new branch if new signature scheme is added.
	var algo x509.SignatureAlgorithm
	switch tokenReq.SignatureAlgorithm {
	case minter.SignatureAlgorithm_SHA256_RSA_ALGO:
		algo = x509.SHA256WithRSA
	default:
		return r.mintingErrorResponse(
			c, minter.ErrorCode_UNSUPPORTED_SIGNATURE,
			"signature_algorithm %s is not supported", tokenReq.SignatureAlgorithm)
	}
	err = cert.CheckSignature(algo, req.SerializedTokenRequest, req.Signature)
	if err != nil {
		return r.mintingErrorResponse(
			c, minter.ErrorCode_BAD_SIGNATURE,
			"signature verification failed - %s", err)
	}

	// At this point we know the request was signed by the holder of a private key
	// that matches the certificate.
	//
	// Let's make sure the token server knows about that key, i.e. the certificate
	// itself is signed by some trusted CA, it is valid (not expired), and it
	// hasn't been revoked yet. CheckCertificate does these checks.
	ca, err := r.CheckCertificate(c, cert)

	// Recognize error codes related to CA cert checking. Everything else is
	// transient errors.
	if err != nil {
		if certchecker.IsCertInvalidError(err) {
			return r.mintingErrorResponse(c, minter.ErrorCode_UNTRUSTED_CERTIFICATE, "%s", err)
		}
		return nil, grpc.Errorf(codes.Internal, "failed to check the certificate - %s", err)
	}

	// At this point we trust what's in MachineTokenRequest, proceed with
	// generating the token.
	args := mintTokenArgs{
		Config:  ca.ParsedConfig,
		Cert:    cert,
		Request: &tokenReq,
	}
	switch tokenReq.TokenType {
	case tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN:
		return r.mintLuciMachineToken(c, args)
	default:
		panic("impossible") // there's a check above
	}
}

type mintTokenArgs struct {
	Config  *admin.CertificateAuthorityConfig
	Cert    *x509.Certificate
	Request *minter.MachineTokenRequest
}

func (r *MintMachineTokenRPC) mintLuciMachineToken(c context.Context, args mintTokenArgs) (*minter.MintMachineTokenResponse, error) {
	// Validate FQDN and whether it is allowed by config.
	params := MintParams{
		FQDN:   strings.ToLower(args.Cert.Subject.CommonName),
		Cert:   args.Cert,
		Config: args.Config,
		Signer: r.Signer,
	}
	if err := params.Validate(); err != nil {
		return r.mintingErrorResponse(c, minter.ErrorCode_BAD_TOKEN_ARGUMENTS, "%s", err)
	}

	serviceVer, err := r.serviceVersion(c)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't grab service version - %s", err)
	}

	// Make the token.
	switch body, signedToken, err := Mint(c, params); {
	case err == nil:
		expiry := time.Unix(int64(body.IssuedAt), 0).Add(time.Duration(body.Lifetime) * time.Second)
		return &minter.MintMachineTokenResponse{
			ServiceVersion: serviceVer,
			TokenResponse: &minter.MachineTokenResponse{
				ServiceVersion: serviceVer,
				TokenType: &minter.MachineTokenResponse_LuciMachineToken{
					LuciMachineToken: &minter.LuciMachineToken{
						MachineToken: signedToken,
						Expiry:       google.NewTimestamp(expiry),
					},
				},
			},
		}, nil
	case errors.IsTransient(err):
		return nil, grpc.Errorf(codes.Internal, "failed to generate machine token - %s", err)
	default:
		return r.mintingErrorResponse(c, minter.ErrorCode_MACHINE_TOKEN_MINTING_ERROR, "%s", err)
	}
}

func (r *MintMachineTokenRPC) mintingErrorResponse(c context.Context, code minter.ErrorCode, msg string, args ...interface{}) (*minter.MintMachineTokenResponse, error) {
	serviceVer, err := r.serviceVersion(c)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't grab service version - %s", err)
	}
	return &minter.MintMachineTokenResponse{
		ErrorCode:      code,
		ErrorMessage:   fmt.Sprintf(msg, args...),
		ServiceVersion: serviceVer,
	}, nil
}
