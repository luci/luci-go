// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"

	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

const delegationCfg = "delegation.cfg"

// ImportDelegationConfigsRPC implements Admin.ImportDelegationConfigs method.
type ImportDelegationConfigsRPC struct {
}

// ImportDelegationConfigs fetches configs from from luci-config right now.
func (r *ImportDelegationConfigsRPC) ImportDelegationConfigs(c context.Context, _ *google.Empty) (*admin.ImportedConfigs, error) {
	cfg, err := fetchConfigFile(c, delegationCfg)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't read config file - %s", err)
	}
	logging.Infof(c, "Importing %q at rev %s", delegationCfg, cfg.Revision)

	// This is returned on successful import.
	successResp := &admin.ImportedConfigs{
		ImportedConfigs: []*admin.ImportedConfigs_ConfigFile{
			{
				Name:     delegationCfg,
				Revision: cfg.Revision,
			},
		},
	}

	// Already have this revision in the datastore?
	existing, err := FetchDelegationConfig(c)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't read existing config - %s", err)
	}
	if existing.Revision == cfg.Revision {
		logging.Infof(c, "Up-to-date at rev %s", cfg.Revision)
		return successResp, nil
	}

	// Validate the new config before storing.
	msg := &admin.DelegationPermissions{}
	if err = proto.UnmarshalText(cfg.Content, msg); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "can't parse config file - %s", err)
	}
	if merr := ValidateConfig(msg); len(merr) != 0 {
		logging.Errorf(c, "The config at rev %s is invalid: %s", cfg.Revision, merr)
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
		Revision:     cfg.Revision,
		Config:       blob,
		ParsedConfig: msg,
	}
	if err := ds.Put(c, &imported); err != nil {
		return nil, grpc.Errorf(codes.Internal, "failed to store the config - %s", err)
	}

	logging.Infof(c, "Updated delegation config %s => %s", existing.Revision, imported.Revision)
	return successResp, nil
}

// fetchConfigFile fetches a file from this services' config set.
func fetchConfigFile(c context.Context, path string) (*config.Config, error) {
	configSet := "services/" + info.AppID(c)
	logging.Infof(c, "Reading %q from config set %q", path, configSet)
	c, _ = context.WithTimeout(c, 30*time.Second) // URL fetch deadline
	return config.GetConfig(c, configSet, path, false)
}
