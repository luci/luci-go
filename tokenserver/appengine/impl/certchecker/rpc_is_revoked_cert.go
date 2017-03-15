// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package certchecker

import (
	"math/big"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

// IsRevokedCertRPC implements CertificateAuthorities.IsRevokedCert RPC method.
type IsRevokedCertRPC struct {
}

// IsRevokedCert says whether a certificate serial number is in the CRL.
func (r *IsRevokedCertRPC) IsRevokedCert(c context.Context, req *admin.IsRevokedCertRequest) (*admin.IsRevokedCertResponse, error) {
	sn := big.Int{}
	if _, ok := sn.SetString(req.Sn, 0); !ok {
		return nil, grpc.Errorf(codes.InvalidArgument, "can't parse 'sn'")
	}

	checker, err := GetCertChecker(c, req.Ca)
	if err != nil {
		if details, ok := err.(Error); ok && details.Reason == NoSuchCA {
			return nil, grpc.Errorf(codes.NotFound, "no such CA: %q", req.Ca)
		}
		return nil, grpc.Errorf(codes.Internal, "failed to check %q CRL - %s", req.Ca, err)
	}

	revoked, err := checker.CRL.IsRevokedSN(c, &sn)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "failed to check %q CRL - %s", req.Ca, err)
	}

	return &admin.IsRevokedCertResponse{Revoked: revoked}, nil
}
