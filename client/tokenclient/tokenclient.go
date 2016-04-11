// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tokenclient

import (
	"crypto/x509"
	"fmt"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/proto/google"

	"github.com/luci/luci-go/common/api/tokenserver/v1"
)

// Client can make signed requests to the token server.
type Client struct {
	// Client is interface to use for raw RPC calls to the token server.
	//
	// Use tokenserver.NewTokenMinterClient (or NewTokenMinterPRPCClient) to
	// create it. Note that transport-level authentication is not needed.
	Client tokenserver.TokenMinterClient

	// Signer knows how to sign requests using some private key.
	Signer Signer
}

// Signer knows how to sign requests using some private key.
type Signer interface {
	// Algo returns an algorithm that the signer implements.
	Algo(ctx context.Context) (x509.SignatureAlgorithm, error)

	// Certificate returns ASN.1 DER blob with the certificate of the signer.
	Certificate(ctx context.Context) ([]byte, error)

	// Sign signs a blob using the private key.
	Sign(ctx context.Context, blob []byte) ([]byte, error)
}

// MintToken signs the request using the signer and sends it.
//
// It will update in-place the following fields of the request:
//   * Certificate will be set to ASN1 cert corresponding to the signer key.
//   * SignatureAlgorithm will be set to the algorithm used to sign the request.
//   * IssuedAt will be set to the current time.
//
// The rest of the fields must be already populated by the caller and will be
// sent to the server as is.
//
// Returns:
//   * TokenResponse on success.
//   * Non-transient error on fatal errors.
//   * Transient error on transient errors.
func (c *Client) MintToken(ctx context.Context, req *tokenserver.TokenRequest, opts ...grpc.CallOption) (*tokenserver.TokenResponse, error) {
	// Fill in SignatureAlgorithm.
	algo, err := c.Signer.Algo(ctx)
	if err != nil {
		return nil, err
	}
	switch algo {
	case x509.SHA256WithRSA:
		req.SignatureAlgorithm = tokenserver.TokenRequest_SHA256_RSA_ALGO
	default:
		return nil, fmt.Errorf("unsupported signing algorithm - %s", algo)
	}

	// Fill in Certificate and IssuedAt.
	if req.Certificate, err = c.Signer.Certificate(ctx); err != nil {
		return nil, err
	}
	req.IssuedAt = google.NewTimestamp(clock.Now(ctx))

	// Serialize and sign.
	tokenRequest, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	signature, err := c.Signer.Sign(ctx, tokenRequest)
	if err != nil {
		return nil, err
	}

	// Make an RPC call (with retries done by pRPC client).
	resp, err := c.Client.MintToken(ctx, &tokenserver.MintTokenRequest{
		SerializedTokenRequest: tokenRequest,
		Signature:              signature,
	}, opts...)

	// Fatal pRPC-level error or transient error in case retries didn't help.
	if err != nil {
		if grpcutil.IsTransient(err) {
			err = errors.WrapTransient(err)
		}
		return nil, err
	}

	// The response still may indicate a fatal error.
	if resp.ErrorCode != tokenserver.MintTokenResponse_SUCCESS {
		details := resp.ErrorMessage
		if details == "" {
			details = "no detailed error message"
		}
		return nil, fmt.Errorf("token server error %s - %s", resp.ErrorCode, details)
	}

	// Must not happen. But better return an error than nil-panic if it does.
	if resp.TokenResponse == nil {
		return nil, fmt.Errorf("token server didn't return a token")
	}

	return resp.TokenResponse, nil
}
