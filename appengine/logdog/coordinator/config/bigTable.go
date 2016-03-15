// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	gaeauthClient "github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/logdog/storage"
	"github.com/luci/luci-go/server/logdog/storage/bigtable"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
	"google.golang.org/grpc/metadata"
)

// GetStorage returns a configured BigTable Storage instance.
//
// The instance is configured from the configuration returned by Get. Upon
// success, the returned instance will need to be Close()'d when the caller has
// finished with it.
func GetStorage(c context.Context) (storage.Storage, error) {
	gcfg, err := LoadGlobalConfig(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load global configuration.")
		return nil, err
	}

	cfg, err := gcfg.LoadConfig(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load instance configuration.")
		return nil, err
	}

	// Is BigTable configured?
	if cfg.Storage == nil {
		return nil, errors.New("no storage configuration")
	}

	bt := cfg.Storage.GetBigtable()
	if bt == nil {
		return nil, errors.New("no BigTable configuration")
	}

	// Validate the BigTable configuration.
	log.Fields{
		"project":      bt.Project,
		"zone":         bt.Zone,
		"cluster":      bt.Cluster,
		"logTableName": bt.LogTableName,
	}.Debugf(c, "Connecting to BigTable.")
	var merr errors.MultiError
	if bt.Project == "" {
		merr = append(merr, errors.New("missing project"))
	}
	if bt.Zone == "" {
		merr = append(merr, errors.New("missing zone"))
	}
	if bt.Cluster == "" {
		merr = append(merr, errors.New("missing cluster"))
	}
	if bt.LogTableName == "" {
		merr = append(merr, errors.New("missing log table name"))
	}
	if len(merr) > 0 {
		return nil, merr
	}

	// Get an Authenticator bound to the token scopes that we need for BigTable.
	a, err := gaeauthClient.Authenticator(c, bigtable.StorageScopes, gcfg.BigTableServiceAccountJSON)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create BigTable authenticator.")
		return nil, errors.New("failed to create BigTable authenticator")
	}

	// Explicitly clear gRPC metadata from the Context. It is forwarded to
	// delegate calls by default, and standard request metadata can break BigTable
	// calls.
	c = metadata.NewContext(c, nil)

	return bigtable.New(c, bigtable.Options{
		Project:  bt.Project,
		Zone:     bt.Zone,
		Cluster:  bt.Cluster,
		LogTable: bt.LogTableName,
		ClientOptions: []cloud.ClientOption{
			cloud.WithTokenSource(a.TokenSource()),
		},
	}), nil
}
