// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package serviceaccounts implements ServiceAccounts API.
//
// Code defined here is either invoked by an administrator or by the service
// itself (from implementation of other services).
package serviceaccounts

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/errors"

	"github.com/luci/luci-go/appengine/cmd/tokenserver/certchecker"
	"github.com/luci/luci-go/appengine/cmd/tokenserver/model"
	"github.com/luci/luci-go/common/api/tokenserver/v1"
)

// Server implements tokenserver.ServiceAccountsServer RPC interface.
//
// It assumes authorization has happened already.
type Server struct {
}

// CreateServiceAccount creates Google Cloud IAM service account associated
// with given CN.
func (s *Server) CreateServiceAccount(c context.Context, r *tokenserver.CreateServiceAccountRequest) (*tokenserver.CreateServiceAccountResponse, error) {
	// Grab a CA config cached inside CertChecker.
	var ca *model.CA
	checker, err := certchecker.GetCertChecker(c, r.Ca)
	if err == nil {
		ca, err = checker.GetCA(c)
	}
	switch {
	case errors.IsTransient(err):
		return nil, grpc.Errorf(codes.Internal, "transient error when fetching CA config - %s", err)
	case err != nil:
		return nil, grpc.Errorf(codes.Unknown, "can't fetch CA config - %s", err)
	}
	cfg := ca.ParsedConfig

	// TODO(vadimsh): Do stuff.
	_ = cfg

	return nil, grpc.Errorf(codes.Unimplemented, "TODO: implement")
}
