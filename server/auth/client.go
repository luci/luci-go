// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"net/http"

	"golang.org/x/net/context"
	"google.golang.org/grpc/credentials"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/server/auth/internal"
)

// RPCAuthorityKind defines under whose authority RPCs are made.
type RPCAuthorityKind int

const (
	// NoAuth is used for outbound RPCs that don't have any implicit auth headers.
	NoAuth RPCAuthorityKind = iota

	// AsSelf is used for outbound RPCs sent with the authority of the current
	// service itself.
	//
	// RPC requests done in this mode will have 'Authorization' header set to the
	// OAuth2 access token of the service's own service account.
	//
	// By default uses "https://www.googleapis.com/auth/userinfo.email" API scope.
	// Can be customized with WithScopes() options.
	AsSelf

	// AsUser is used for outbound RPCs that inherit the authority of a user
	// that initiated the request that is currently being handled.
	//
	// It is based on LUCI-specific protocol that uses special delegation tokens.
	// Only LUCI backends can understand them.
	AsUser
)

// RPCOption is an option for GetRPCTransport or GetPerRPCCredentials functions.
type RPCOption interface {
	apply(opts *rpcOptions)
}

// WithScopes can be used to customize OAuth scopes for outbound RPC requests.
//
// If not used, the requests are made with "userinfo.email" scope.
func WithScopes(scopes ...string) RPCOption {
	return oauthScopesOption{scopes: scopes}
}

type oauthScopesOption struct {
	scopes []string
}

func (o oauthScopesOption) apply(opts *rpcOptions) {
	opts.scopes = append(opts.scopes, o.scopes...)
}

// GetRPCTransport returns http.RoundTripper to use for outbound HTTP RPC
// requests.
//
// Usage:
//    tr, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes("..."))
//    if err != nil {
//      return err
//    }
//    client := &http.Client{Transport: tr}
//    ...
func GetRPCTransport(c context.Context, kind RPCAuthorityKind, opts ...RPCOption) (http.RoundTripper, error) {
	options, err := makeRpcOptions(kind, opts)
	if err != nil {
		return nil, err
	}
	config := GetConfig(c)
	if config == nil || config.AnonymousTransport == nil {
		return nil, ErrNotConfigured
	}
	baseTransport := config.AnonymousTransport(c)
	if options.kind == NoAuth {
		return baseTransport, nil
	}
	return auth.NewModifyingTransport(baseTransport, func(req *http.Request) error {
		headers, err := options.getRPCHeaders(c, options)
		if err != nil {
			return err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return nil
	}), nil
}

// GetPerRPCCredentials returns gRPC's PerRPCCredentials implementation.
//
// It can be used to authenticate outbound gPRC RPC's.
func GetPerRPCCredentials(kind RPCAuthorityKind, opts ...RPCOption) (credentials.PerRPCCredentials, error) {
	options, err := makeRpcOptions(kind, opts)
	if err != nil {
		return nil, err
	}
	return perRPCCreds{options}, nil
}

type perRPCCreds struct {
	options *rpcOptions
}

func (creds perRPCCreds) GetRequestMetadata(c context.Context, uri ...string) (map[string]string, error) {
	return creds.options.getRPCHeaders(c, creds.options)
}

func (creds perRPCCreds) RequireTransportSecurity() bool {
	return true
}

////////////////////////////////////////////////////////////////////////////////
// Internal stuff.

func init() {
	// This is needed to allow packages imported by 'server/auth' to make
	// authenticated calls. They can't use GetRPCTransport directly, since they
	// can't import 'server/auth' (it creates an import cycle).
	internal.RegisterClientFactory(func(c context.Context, scopes []string) (*http.Client, error) {
		var t http.RoundTripper
		var err error
		if len(scopes) == 0 {
			t, err = GetRPCTransport(c, NoAuth)
		} else {
			t, err = GetRPCTransport(c, AsSelf, WithScopes(scopes...))
		}
		if err != nil {
			return nil, err
		}
		return &http.Client{Transport: t}, nil
	})
}

var defaultOAuthScopes = []string{auth.OAuthScopeEmail}

type headersGetter func(c context.Context, opts *rpcOptions) (map[string]string, error)

type rpcOptions struct {
	kind          RPCAuthorityKind
	scopes        []string
	getRPCHeaders headersGetter
}

// makeRpcOptions applies all options and validates them.
func makeRpcOptions(kind RPCAuthorityKind, opts []RPCOption) (*rpcOptions, error) {
	options := &rpcOptions{kind: kind}
	for _, o := range opts {
		o.apply(options)
	}
	// Validate scopes.
	switch {
	case options.kind == AsSelf && len(options.scopes) == 0:
		options.scopes = defaultOAuthScopes
	case options.kind != AsSelf && len(options.scopes) != 0:
		return nil, fmt.Errorf("auth: WithScopes can only be used with AsSelf authorization kind")
	}
	// Validate 'kind' and pick correct implementation of getRPCHeaders.
	switch options.kind {
	case NoAuth:
		options.getRPCHeaders = noAuthHeaders
	case AsSelf:
		options.getRPCHeaders = asSelfHeaders
	case AsUser:
		return nil, fmt.Errorf("auth: AsUser RPCAuthorityKind is not implemented")
	default:
		return nil, fmt.Errorf("auth: unknown RPCAuthorityKind %d", options.kind)
	}
	return options, nil
}

// noAuthHeaders is getRPCHeaders for NoAuth mode.
func noAuthHeaders(c context.Context, opts *rpcOptions) (map[string]string, error) {
	return nil, nil
}

// asSelfHeaders returns a map of authentication headers to add to outbound
// RPC requests done in AsSelf mode.
//
// This will be called by the transport layer on each request.
func asSelfHeaders(c context.Context, opts *rpcOptions) (map[string]string, error) {
	cfg := GetConfig(c)
	if cfg == nil || cfg.AccessTokenProvider == nil {
		return nil, ErrNotConfigured
	}
	tok, err := cfg.AccessTokenProvider(c, opts.scopes)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"Authorization": tok.TokenType + " " + tok.AccessToken,
	}, nil
}
