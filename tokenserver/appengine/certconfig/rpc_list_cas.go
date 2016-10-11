// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package certconfig

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	ds "github.com/luci/gae/service/datastore"

	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

// ListCAsRPC implements CertificateAuthorities.ListCAs RPC method.
type ListCAsRPC struct {
}

// ListCAs returns a list of Common Names of registered CAs.
func (r *ListCAsRPC) ListCAs(c context.Context, _ *google.Empty) (*admin.ListCAsResponse, error) {
	keys := []*ds.Key{}

	q := ds.NewQuery("CA").Eq("Removed", false).KeysOnly(true)
	if err := ds.GetAll(c, q, &keys); err != nil {
		return nil, grpc.Errorf(codes.Internal, "transient datastore error - %s", err)
	}

	resp := &admin.ListCAsResponse{
		Cn: make([]string, len(keys)),
	}
	for i, key := range keys {
		resp.Cn[i] = key.StringID()
	}
	return resp, nil
}
