// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	ds "github.com/luci/gae/service/datastore"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/textproto"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

const delegationCfg = "delegation.cfg"

// ImportDelegationConfigsRPC implements Admin.ImportDelegationConfigs method.
type ImportDelegationConfigsRPC struct {
}

// ImportDelegationConfigs fetches configs from from luci-config right now.
func (r *ImportDelegationConfigsRPC) ImportDelegationConfigs(c context.Context, _ *empty.Empty) (*admin.ImportedConfigs, error) {
	msg, meta, err := fetchConfigDelegationPermissions(c, delegationCfg)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't read config file - %s", err)
	}
	logging.Infof(c, "Importing %q at rev %s", delegationCfg, meta.Revision)

	// Already have this revision in the datastore?
	existing, err := FetchDelegationConfig(c)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't read existing config - %s", err)
	}
	if existing.Revision == meta.Revision {
		logging.Infof(c, "Up-to-date at rev %s", meta.Revision)
		return &admin.ImportedConfigs{Revision: meta.Revision}, nil
	}

	// Validate the new config before storing.
	if merr := ValidateConfig(msg); len(merr) != 0 {
		logging.Errorf(c, "The config at rev %s is invalid: %s", meta.Revision, merr)
		for _, err := range merr {
			logging.Errorf(c, "%s", err)
		}
		return nil, grpc.Errorf(codes.InvalidArgument, "validation error - %s", merr)
	}

	// Success!
	blob, err := proto.Marshal(msg)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't serialize proto - %s", err)
	}
	imported := DelegationConfig{
		Revision:     meta.Revision,
		Config:       blob,
		ParsedConfig: msg,
	}
	if err := ds.Put(c, &imported); err != nil {
		return nil, grpc.Errorf(codes.Internal, "failed to store the config - %s", err)
	}

	logging.Infof(c, "Updated delegation config %s => %s", existing.Revision, imported.Revision)
	return &admin.ImportedConfigs{Revision: meta.Revision}, nil
}

// fetchConfigDelegationPermissions fetches a file from this services' config set.
func fetchConfigDelegationPermissions(c context.Context, path string) (*admin.DelegationPermissions, *cfgclient.Meta, error) {
	configSet := cfgclient.CurrentServiceConfigSet(c)
	logging.Infof(c, "Reading %q from config set %q", path, configSet)
	c, cancelFunc := context.WithTimeout(c, 30*time.Second) // URL fetch deadline
	defer cancelFunc()

	var (
		meta cfgclient.Meta
		msg  admin.DelegationPermissions
	)
	if err := cfgclient.Get(c, cfgclient.AsService, configSet, path, textproto.Message(&msg), &meta); err != nil {
		return nil, nil, err
	}
	return &msg, &meta, nil
}
