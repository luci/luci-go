// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tokenclient

import (
	"net/http"

	"github.com/luci/luci-go/common/prpc"
	"github.com/luci/luci-go/common/retry"

	"github.com/luci/luci-go/common/api/tokenserver/minter/v1"
)

// ClientParameters is passed to NewClient.
type ClientParameters struct {
	// PrivateKeyPath is a path to a file with a private key PEM file.
	//
	// Required.
	PrivateKeyPath string

	// CertificatePath is a path to a file with a corresponding certificate.
	//
	// Required. It must match the private key (this will be verified).
	CertificatePath string

	// Backend is a hostname of the token server to talk to.
	//
	// Required.
	Backend string

	// Insecure is true to use 'http' protocol instead of 'https'.
	//
	// Useful on localhost. Default is "secure".
	Insecure bool

	// Client is non-authenticating HTTP client to build pRPC transport on top of.
	//
	// Default is http.DefaultClient.
	Client *http.Client

	// Retry defines how to retry RPC requests on transient errors.
	//
	// Use retry.Default for default strategy. Default is "no retries".
	Retry retry.Factory
}

// NewClient returns new Client that uses PEM encoded keys and talks
// to the server via pRPC.
func NewClient(params ClientParameters) (*Client, error) {
	signer, err := LoadX509Signer(params.PrivateKeyPath, params.CertificatePath)
	if err != nil {
		return nil, err
	}
	return &Client{
		Client: minter.NewTokenMinterPRPCClient(&prpc.Client{
			C:    params.Client,
			Host: params.Backend,
			Options: &prpc.Options{
				Retry:    params.Retry,
				Insecure: params.Insecure,
			},
		}),
		Signer: signer,
	}, nil
}
