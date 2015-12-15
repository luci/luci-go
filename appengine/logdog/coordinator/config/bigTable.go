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
)

// GetStorage returns a configured BigTable Storage instance.
//
// The instance is configured from the configuration returned by Get. Upon
// success, the returned instance will need to be Close()'d when the caller has
// finished with it.
func GetStorage(c context.Context) (storage.Storage, error) {
	cfg, err := Get(c)
	if err != nil {
		return nil, err
	}

	// Is BigTable configured?
	bt := cfg.Bigtable
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
	auth, err := gaeauthClient.Authenticator(c, bigtable.StorageReadOnlyScopes, nil)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to create BigTable authenticator.")
		return nil, errors.New("failed to create BigTable authenticator")
	}

	return bigtable.New(c, bigtable.Options{
		Project:  bt.Project,
		Zone:     bt.Zone,
		Cluster:  bt.Cluster,
		LogTable: bt.LogTableName,
		ClientOptions: []cloud.ClientOption{
			cloud.WithTokenSource(auth.TokenSource()),
		},
	})
}
