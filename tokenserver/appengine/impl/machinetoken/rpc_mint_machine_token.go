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
	"crypto/x509"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"

	tokenserver "go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/certchecker"
	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
)

// MintMachineTokenRPC implements TokenMinter.MintMachineToken RPC method.
type MintMachineTokenRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is the default server signer that uses server's service account.
	Signer signing.Signer

	// CheckCertificate is mocked in tests.
	//
	// In prod it is certchecker.CheckCertificate.
	CheckCertificate func(c context.Context, cert *x509.Certificate) (*certconfig.CA, error)

	// LogToken is mocked in tests.
	//
	// In prod it is produced by NewTokenLogger.
	LogToken TokenLogger
}

// MintMachineToken generates a new token for an authenticated machine.
func (r *MintMachineTokenRPC) MintMachineToken(c context.Context, req *minter.MintMachineTokenRequest) (*minter.MintMachineTokenResponse, error) {
	// Parse serialized portion of the request and do minimal validation before
	// checking the signature to reject obviously bad requests.
	if len(req.SerializedTokenRequest) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}
	tokenReq := minter.MachineTokenRequest{}
	if err := proto.Unmarshal(req.SerializedTokenRequest, &tokenReq); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to unmarshal TokenRequest - %s", err)
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
	issuedAt := tokenReq.IssuedAt.AsTime()
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
		return nil, status.Errorf(codes.Internal, "failed to check the certificate - %s", err)
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
		resp, body, err := r.mintLuciMachineToken(c, args)
		switch {
		case err != nil: // grpc-level error
			return nil, err
		case resp == nil: // should not happen
			panic("both resp and err can't be nil")
		case resp.ErrorCode != 0: // logic-level error
			if resp.TokenResponse != nil {
				panic("TokenResponse must be nil if ErrorCode != 0")
			}
			return resp, nil
		}
		if resp.TokenResponse == nil {
			panic("TokenResponse must not be nil if ErrorCode == 0")
		}
		if r.LogToken != nil {
			// Errors during logging are considered not fatal. We have a monitoring
			// counter that tracks number of errors, so they are not totally
			// invisible.
			tokInfo := MintedTokenInfo{
				Request:   &tokenReq,
				Response:  resp.TokenResponse,
				TokenBody: body,
				CA:        ca,
				PeerIP:    auth.GetState(c).PeerIP(),
				RequestID: trace.SpanContextFromContext(c).TraceID().String(),
			}
			if logErr := r.LogToken(c, &tokInfo); logErr != nil {
				logging.WithError(logErr).Errorf(c, "Failed to insert the machine token into BigQuery log")
			}
		}
		return resp, nil
	default:
		panic("impossible") // there's a check above
	}
}

type mintTokenArgs struct {
	Config  *admin.CertificateAuthorityConfig
	Cert    *x509.Certificate
	Request *minter.MachineTokenRequest
}

func (r *MintMachineTokenRPC) mintLuciMachineToken(c context.Context, args mintTokenArgs) (*minter.MintMachineTokenResponse, *tokenserver.MachineTokenBody, error) {
	// Validate FQDN and whether it is allowed by config. The FQDN is extracted
	// from the cert.
	params := MintParams{
		Cert:   args.Cert,
		Config: args.Config,
		Signer: r.Signer,
	}
	if err := params.Validate(); err != nil {
		resp, err := r.mintingErrorResponse(c, minter.ErrorCode_BAD_TOKEN_ARGUMENTS, "%s", err)
		return resp, nil, err
	}

	serviceVer, err := utils.ServiceVersion(c, r.Signer)
	if err != nil {
		return nil, nil, status.Errorf(codes.Internal, "can't grab service version - %s", err)
	}

	// Make the token.
	switch body, signedToken, err := Mint(c, &params); {
	case err == nil:
		expiry := time.Unix(int64(body.IssuedAt), 0).Add(time.Duration(body.Lifetime) * time.Second)
		return &minter.MintMachineTokenResponse{
			ServiceVersion: serviceVer,
			TokenResponse: &minter.MachineTokenResponse{
				ServiceVersion: serviceVer,
				TokenType: &minter.MachineTokenResponse_LuciMachineToken{
					LuciMachineToken: &minter.LuciMachineToken{
						MachineToken: signedToken,
						Expiry:       timestamppb.New(expiry),
					},
				},
			},
		}, body, nil
	case transient.Tag.In(err):
		return nil, nil, status.Errorf(codes.Internal, "failed to generate machine token - %s", err)
	default:
		resp, err := r.mintingErrorResponse(c, minter.ErrorCode_MACHINE_TOKEN_MINTING_ERROR, "%s", err)
		return resp, nil, err
	}
}

func (r *MintMachineTokenRPC) mintingErrorResponse(c context.Context, code minter.ErrorCode, msg string, args ...any) (*minter.MintMachineTokenResponse, error) {
	serviceVer, err := utils.ServiceVersion(c, r.Signer)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't grab service version - %s", err)
	}
	return &minter.MintMachineTokenResponse{
		ErrorCode:      code,
		ErrorMessage:   fmt.Sprintf(msg, args...),
		ServiceVersion: serviceVer,
	}, nil
}
