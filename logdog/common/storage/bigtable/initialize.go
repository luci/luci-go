// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigtable

import (
	"fmt"
	"time"

	"cloud.google.com/go/bigtable"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/logdog/common/storage"
	"golang.org/x/net/context"
)

// DefaultMaxLogAge is the maximum age of a log (7 days).
const DefaultMaxLogAge = time.Duration(7 * 24 * time.Hour)

// InitializeScopes is the set of OAuth scopes needed to use the Initialize
// functionality.
var InitializeScopes = []string{
	bigtable.AdminScope,
}

func tableExists(c context.Context, ac *bigtable.AdminClient, name string) (bool, error) {
	tables, err := ac.Tables(c)
	if err != nil {
		return false, err
	}

	for _, t := range tables {
		if t == name {
			return true, nil
		}
	}
	return false, nil
}

func waitForTable(c context.Context, ac *bigtable.AdminClient, name string) error {
	return retry.Retry(c, transient.Only(retry.Default), func() error {
		exists, err := tableExists(c, ac, name)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("table does not exist", transient.Tag)
		}
		return nil
	}, func(err error, delay time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      delay,
		}.Warningf(c, "Table does not exist yet; retrying.")
	})
}

// Initialize sets up a Storage schema in BigTable. If the schema is already
// set up properly, no action will be taken.
//
// If, however, the table or table's schema doesn't exist, Initialize will
// create and configure it.
//
// If nil is returned, the table is ready for use as a Storage via New.
func (s *Storage) Initialize(c context.Context) error {
	if s.AdminClient == nil {
		return errors.New("no admin client configured")
	}

	exists, err := tableExists(c, s.AdminClient, s.LogTable)
	if err != nil {
		return fmt.Errorf("failed to test for table: %s", err)
	}
	if !exists {
		log.Fields{
			"table": s.LogTable,
		}.Infof(c, "Storage table does not exist. Creating...")

		if err := s.AdminClient.CreateTable(c, s.LogTable); err != nil {
			return fmt.Errorf("failed to create table: %s", err)
		}

		// Wait for the table to exist. BigTable API says this can be delayed from
		// creation.
		if err := waitForTable(c, s.AdminClient, s.LogTable); err != nil {
			return fmt.Errorf("failed to wait for table to exist: %s", err)
		}

		log.Fields{
			"table": s.LogTable,
		}.Infof(c, "Successfully created storage table.")
	}

	// Get table info.
	ti, err := s.AdminClient.TableInfo(c, s.LogTable)
	if err != nil {
		return fmt.Errorf("failed to get table info: %s", err)
	}

	// The table must have the "log" column family.
	families := stringset.NewFromSlice(ti.Families...)
	if !families.Has(logColumnFamily) {
		log.Fields{
			"table":  s.LogTable,
			"family": logColumnFamily,
		}.Infof(c, "Column family 'log' does not exist. Creating...")

		// Create the logColumnFamily column family.
		if err := s.AdminClient.CreateColumnFamily(c, s.LogTable, logColumnFamily); err != nil {
			return fmt.Errorf("Failed to create 'log' column family: %s", err)
		}

		log.Fields{
			"table":  s.LogTable,
			"family": "log",
		}.Infof(c, "Successfully created 'log' column family.")
	}

	cfg := storage.Config{
		MaxLogAge: DefaultMaxLogAge,
	}
	if err := s.Config(c, cfg); err != nil {
		log.WithError(err).Errorf(c, "Failed to push default configuration.")
		return err
	}

	return nil
}
