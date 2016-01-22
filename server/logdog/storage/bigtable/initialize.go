// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"fmt"
	"time"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"github.com/luci/luci-go/common/stringset"
	"golang.org/x/net/context"
	"google.golang.org/cloud/bigtable"
)

// MaxLogAge is the maximum age of a log (7 days).
const MaxLogAge = time.Duration(7 * 24 * time.Hour)

// InitializeScopes is the set of OAuth scopes needed to use the Initialize
// functionality.
var InitializeScopes = []string{
	bigtable.AdminScope,
}

func tableExists(ctx context.Context, c *bigtable.AdminClient, name string) (bool, error) {
	tables, err := c.Tables(ctx)
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

func waitForTable(ctx context.Context, c *bigtable.AdminClient, name string) error {
	return retry.Retry(ctx, retry.TransientOnly(retry.Default), func() error {
		exists, err := tableExists(ctx, c, name)
		if err != nil {
			return err
		}
		if !exists {
			return errors.WrapTransient(errors.New("table does not exist"))
		}
		return nil
	}, func(err error, delay time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      delay,
		}.Warningf(ctx, "Table does not exist yet; retrying.")
	})
}

// Initialize sets up a Storage schema in BigTable. If the schema is already
// set up properly, no action will be taken.
//
// If, however, the table or table's schema doesn't exist, Initialize will
// create and configure it.
//
// If nil is returned, the table is ready for use as a Storage via New.
func Initialize(ctx context.Context, o Options) error {
	c, err := bigtable.NewAdminClient(ctx, o.Project, o.Zone, o.Cluster, o.ClientOptions...)
	if err != nil {
		return fmt.Errorf("failed to create admin client: %s", err)
	}
	defer c.Close()

	exists, err := tableExists(ctx, c, o.LogTable)
	if err != nil {
		return fmt.Errorf("failed to test for table: %s", err)
	}
	if !exists {
		log.Fields{
			"table": o.LogTable,
		}.Infof(ctx, "Storage table does not exist. Creating...")

		if err := c.CreateTable(ctx, o.LogTable); err != nil {
			return fmt.Errorf("failed to create table: %s", err)
		}

		// Wait for the table to exist. BigTable API says this can be delayed from
		// creation.
		if err := waitForTable(ctx, c, o.LogTable); err != nil {
			return fmt.Errorf("failed to wait for table to exist: %s", err)
		}

		log.Fields{
			"table": o.LogTable,
		}.Infof(ctx, "Successfully created storage table.")
	}

	// Get table info.
	ti, err := c.TableInfo(ctx, o.LogTable)
	if err != nil {
		return fmt.Errorf("failed to get table info: %s", err)
	}

	// The table must have the "log" column family.
	families := stringset.NewFromSlice(ti.Families...)
	if !families.Has("log") {
		log.Fields{
			"table":  o.LogTable,
			"family": "log",
		}.Infof(ctx, "Column family 'log' does not exist. Creating...")

		// Create the "log" column family.
		if err := c.CreateColumnFamily(ctx, o.LogTable, "log"); err != nil {
			return fmt.Errorf("Failed to create 'log' column family: %s", err)
		}

		// Set the garbage collection policy.
		if o.EnableGarbageCollection {
			if err := c.SetGCPolicy(ctx, o.LogTable, "log", bigtable.MaxAgePolicy(MaxLogAge)); err != nil {
				return fmt.Errorf("Failed to set 'log' GC policy: %s", err)
			}
		}

		log.Fields{
			"table":  o.LogTable,
			"family": "log",
		}.Infof(ctx, "Successfully created 'log' column family.")
	}

	return nil
}
