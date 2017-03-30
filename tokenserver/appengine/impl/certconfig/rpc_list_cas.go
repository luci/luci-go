// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package certconfig

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/golang/protobuf/ptypes/empty"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

// ListCAsRPC implements CertificateAuthorities.ListCAs RPC method.
type ListCAsRPC struct {
}

// ListCAs returns a list of Common Names of registered CAs.
func (r *ListCAsRPC) ListCAs(c context.Context, _ *empty.Empty) (*admin.ListCAsResponse, error) {
	names, err := ListCAs(c)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "transient datastore error - %s", err)
	}
	return &admin.ListCAsResponse{Cn: names}, nil
}
