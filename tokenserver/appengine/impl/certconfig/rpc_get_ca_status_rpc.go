// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package certconfig

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	ds "github.com/luci/gae/service/datastore"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils"
)

// GetCAStatusRPC implements CertificateAuthorities.GetCAStatus RPC method.
type GetCAStatusRPC struct {
}

// GetCAStatus returns configuration of some CA defined in the config.
func (r *GetCAStatusRPC) GetCAStatus(c context.Context, req *admin.GetCAStatusRequest) (*admin.GetCAStatusResponse, error) {
	// Entities to fetch.
	ca := CA{CN: req.Cn}
	crl := CRL{Parent: ds.KeyForObj(c, &ca)}

	// Fetch them at the same revision. It is fine if CRL is not there yet. Don't
	// bother doing it in parallel: GetCAStatus is used only by admins, manually.
	err := ds.RunInTransaction(c, func(c context.Context) error {
		if err := ds.Get(c, &ca); err != nil {
			return err // can be ErrNoSuchEntity
		}
		if err := ds.Get(c, &crl); err != nil && err != ds.ErrNoSuchEntity {
			return err // only transient errors
		}
		return nil
	}, nil)
	switch {
	case err == ds.ErrNoSuchEntity:
		return &admin.GetCAStatusResponse{}, nil
	case err != nil:
		return nil, grpc.Errorf(codes.Internal, "datastore error - %s", err)
	}

	cfgMsg, err := ca.ParseConfig()
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "broken config in the datastore - %s", err)
	}

	return &admin.GetCAStatusResponse{
		Config:     cfgMsg,
		Cert:       utils.DumpPEM(ca.Cert, "CERTIFICATE"),
		Removed:    ca.Removed,
		Ready:      ca.Ready,
		AddedRev:   ca.AddedRev,
		UpdatedRev: ca.UpdatedRev,
		RemovedRev: ca.RemovedRev,
		CrlStatus:  crl.GetStatusProto(),
	}, nil
}
