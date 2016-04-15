// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tokenminter implements TokenMinter API.
//
// This is main public API of The Token Server.
package tokenminter

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

	"github.com/luci/luci-go/common/api/tokenserver"
	"github.com/luci/luci-go/common/api/tokenserver/admin/v1"
	"github.com/luci/luci-go/common/api/tokenserver/minter/v1"

	"github.com/luci/luci-go/appengine/cmd/tokenserver/certchecker"
	"github.com/luci/luci-go/appengine/cmd/tokenserver/model"
	"github.com/luci/luci-go/appengine/cmd/tokenserver/services/admin/serviceaccounts"
)

// Server implements minter.TokenMinterServer RPC interface.
//
// Use NewServer to make one.
type Server struct {
	// mintAccessToken is mocked in tests.
	//
	// In prod it is serviceaccounts.Server.DoMintAccessToken.
	mintAccessToken func(context.Context, serviceaccounts.MintAccessTokenParams) (*tokenserver.ServiceAccount, *tokenserver.OAuth2AccessToken, error)

	// certChecker is mocked in tests.
	//
	// In prod it is certchecker.CheckCertificate.
	certChecker func(c context.Context, cert *x509.Certificate) (*model.CA, error)
}

// NewServer returns Server configured for real production usage.
func NewServer(sa *serviceaccounts.Server) *Server {
	return &Server{
		mintAccessToken: sa.DoMintAccessToken,
		certChecker:     certchecker.CheckCertificate,
	}
}

// MintToken generates a new token for an authenticated caller.
func (s *Server) MintToken(c context.Context, req *minter.MintTokenRequest) (*minter.MintTokenResponse, error) {
	// Parse serialized portion of the request and do minimal validation before
	// checking the signature to reject obviously bad requests.
	if len(req.SerializedTokenRequest) == 0 {
		return nil, grpc.Errorf(codes.InvalidArgument, "empty request")
	}
	tokenReq := minter.TokenRequest{}
	if err := proto.Unmarshal(req.SerializedTokenRequest, &tokenReq); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "failed to unmarshal TokenRequest - %s", err)
	}

	// Only Google OAuth2 access tokens are supported for now. Use switch to
	// remind us to add more branches if there are more supported token types in
	// the future.
	switch tokenReq.TokenType {
	case minter.TokenRequest_GOOGLE_OAUTH2_ACCESS_TOKEN:
		// supported
	default:
		return errorResponse(
			minter.MintTokenResponse_UNSUPPORTED_TOKEN_TYPE,
			"token_type %s is not supported", tokenReq.TokenType)
	}

	// Timestamp is required.
	issuedAt := tokenReq.IssuedAt.Time()
	if issuedAt.IsZero() {
		return errorResponse(minter.MintTokenResponse_BAD_TIMESTAMP, "issued_at is required")
	}

	// It should be within acceptable range.
	now := clock.Now(c)
	notBefore := now.Add(-10 * time.Minute)
	notAfter := now.Add(10 * time.Minute)
	if issuedAt.Before(notBefore) || issuedAt.After(notAfter) {
		return errorResponse(
			minter.MintTokenResponse_BAD_TIMESTAMP,
			"issued_at timestamp is not within acceptable range, check your clock")
	}

	// The certificate must be valid.
	cert, err := x509.ParseCertificate(tokenReq.Certificate)
	if err != nil {
		return errorResponse(
			minter.MintTokenResponse_BAD_CERTIFICATE_FORMAT,
			"failed to parse the certificate (expecting x509 cert DER)")
	}

	// Check the signature before proceeding. Use switch when picking an algo
	// as a reminder to add a new branch if new signature scheme is added.
	var algo x509.SignatureAlgorithm
	switch tokenReq.SignatureAlgorithm {
	case minter.TokenRequest_SHA256_RSA_ALGO:
		algo = x509.SHA256WithRSA
	default:
		return errorResponse(
			minter.MintTokenResponse_UNSUPPORTED_SIGNATURE,
			"signature_algorithm %s is not supported", tokenReq.SignatureAlgorithm)
	}
	err = cert.CheckSignature(algo, req.SerializedTokenRequest, req.Signature)
	if err != nil {
		return errorResponse(
			minter.MintTokenResponse_BAD_SIGNATURE,
			"signature verification failed - %s", err)
	}

	// At this point we know the request was signed by the holder of a private key
	// that matches the certificate.
	//
	// Let's make sure the token server knows about that key, i.e. the certificate
	// itself is signed by some trusted CA, it is valid (not expired), and it
	// hasn't been revoked yet. CertChecker does these checks.
	ca, err := s.certChecker(c, cert)

	// Recognize error codes related to CA cert checking. Everything else is
	// transient errors.
	if err != nil {
		if certchecker.IsCertInvalidError(err) {
			return errorResponse(minter.MintTokenResponse_UNTRUSTED_CERTIFICATE, "%s", err)
		}
		return nil, grpc.Errorf(codes.Internal, "failed to check the certificate - %s", err)
	}

	// At this point we trust what's in TokenRequest, proceed with generating
	// the token.
	args := mintTokenArgs{
		Config:  ca.ParsedConfig,
		Cert:    cert,
		Request: &tokenReq,
	}
	switch tokenReq.TokenType {
	case minter.TokenRequest_GOOGLE_OAUTH2_ACCESS_TOKEN:
		return s.mintGoogleOAuth2AccessToken(c, args)
	default:
		panic("impossible") // there's a check above
	}
}

type mintTokenArgs struct {
	Config  *admin.CertificateAuthorityConfig
	Cert    *x509.Certificate
	Request *minter.TokenRequest
}

func (s *Server) mintGoogleOAuth2AccessToken(c context.Context, args mintTokenArgs) (*minter.MintTokenResponse, error) {
	// Validate FQDN, scopes, check they are whitelisted (the whitelist is part of
	// the config).
	params := serviceaccounts.MintAccessTokenParams{
		Config: args.Config,
		FQDN:   strings.ToLower(args.Cert.Subject.CommonName),
		Scopes: args.Request.Oauth2Scopes,
	}
	if err := params.Validate(); err != nil {
		return errorResponse(minter.MintTokenResponse_BAD_TOKEN_ARGUMENTS, "%s", err)
	}

	// Grab the token. It returns grpc error already, but we need to convert it
	// to MintTokenResponse, unless it is a transient error (then we pass it
	// through as is, to retain its "transience").
	account, token, err := s.mintAccessToken(c, params)
	switch {
	case err == nil:
		return &minter.MintTokenResponse{
			TokenResponse: &minter.TokenResponse{
				ServiceAccount: account,
				TokenType: &minter.TokenResponse_GoogleOauth2AccessToken{
					GoogleOauth2AccessToken: token,
				},
			},
		}, nil
	case grpc.Code(err) == codes.Internal:
		return nil, err
	default:
		return errorResponse(minter.MintTokenResponse_TOKEN_MINTING_ERROR, "%s", err)
	}
}

func errorResponse(code minter.MintTokenResponse_ErrorCode, msg string, args ...interface{}) (*minter.MintTokenResponse, error) {
	return &minter.MintTokenResponse{
		ErrorCode:    code,
		ErrorMessage: fmt.Sprintf(msg, args...),
	}, nil
}
