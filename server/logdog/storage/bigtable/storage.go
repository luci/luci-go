// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"fmt"

	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
	"google.golang.org/cloud/bigtable"
)

var (
	// StorageScopes is the set of OAuth scopes needed to use the storage
	// functionality.
	StorageScopes = []string{
		bigtable.Scope,
	}

	// StorageReadOnlyScopes is the set of OAuth scopes needed to use the storage
	// functionality.
	StorageReadOnlyScopes = []string{
		bigtable.ReadonlyScope,
	}
)

const (
	// tailRowCount is the size of the block of rows that tail read operations
	// pull from BigTable. This is designed to be large enough for efficient
	// buffering while staying small enough to avoid wasteful reads or
	// excessive in-memory buffering.
	//
	// This is simply the maximum number of rows (limit). The actual number of
	// rows will be further constrained by tailRowMaxSize.
	tailRowCount = 128

	// tailRowMaxSize is the maximum number of bytes of tail row data that will be
	// buffered during Tail row reading.
	tailRowMaxSize = 1024 * 1024 * 16
)

// Options is a set of configuration options for BigTable storage.
type Options struct {
	// Project is the name of the project to connect to.
	Project string
	// Zone is the name of the zone to connect to.
	Zone string
	// Cluster is the name of the cluster to connect to.
	Cluster string
	// ClientOptions are additional client options to use when instantiating the
	// client instance.
	ClientOptions []cloud.ClientOption

	// Table is the name of the BigTable table to use for logs.
	LogTable string
}

// btStorage is a storage.Storage implementation that uses Google Cloud BigTable
// as a backend.
type btStorage struct {
	*Options

	ctx context.Context

	client      *bigtable.Client
	logTable    *bigtable.Table
	adminClient *bigtable.AdminClient

	table btTable
}

// New instantiates a new Storage instance connected to a BigTable cluster.
//
// The returned Storage instance will close the Client when its Close() method
// is called.
func New(ctx context.Context, o Options) storage.Storage {
	st := &btStorage{
		Options: &o,
		ctx:     ctx,
	}
	st.table = &btTableProd{st}
	return st
}

func (s *btStorage) Close() {
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}

	if s.adminClient != nil {
		s.adminClient.Close()
		s.adminClient = nil
	}
}

func (s *btStorage) Config(cfg storage.Config) error {
	if err := s.table.setMaxLogAge(s.ctx, cfg.MaxLogAge); err != nil {
		log.WithError(err).Errorf(s.ctx, "Failed to set 'log' GC policy.")
		return err
	}
	log.Fields{
		"maxLogAge": cfg.MaxLogAge,
	}.Infof(s.ctx, "Set maximum log age.")
	return nil
}

func (s *btStorage) Put(r *storage.PutRequest) error {
	rk := newRowKey(string(r.Path), int64(r.Index))
	ctx := log.SetFields(s.ctx, log.Fields{
		"rowKey": rk,
		"path":   r.Path,
		"index":  r.Index,
		"size":   len(r.Value),
	})
	log.Debugf(ctx, "Adding entry to BigTable.")

	return s.table.putLogData(ctx, rk, r.Value)
}

func (s *btStorage) Get(r *storage.GetRequest, cb storage.GetCallback) error {
	startKey := newRowKey(string(r.Path), int64(r.Index))
	c := log.SetFields(s.ctx, log.Fields{
		"path":        r.Path,
		"index":       r.Index,
		"startRowKey": startKey,
	})

	err := s.table.getLogData(c, startKey, r.Limit, false, func(rk *rowKey, data []byte) error {
		// Does this key match our requested log stream? If not, we've moved past
		// this stream's records and must stop iteration.
		if !rk.sharesPathWith(startKey) {
			return errStop
		}

		// We have a row. Invoke our callback.
		if !cb(types.MessageIndex(rk.index), data) {
			return errStop
		}
		return nil
	})
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    s.Project,
			"zone":       s.Zone,
			"cluster":    s.Cluster,
			"table":      s.LogTable,
		}.Errorf(c, "Failed to retrieve row range.")
		return err
	}
	return nil
}

func (s *btStorage) Tail(p types.StreamPath) ([]byte, types.MessageIndex, error) {
	c := log.SetFields(s.ctx, log.Fields{
		"path": p,
	})

	// Iterate through all log keys in the stream. Record the latest one.
	rk := newRowKey(string(p), 0)
	var latest *rowKey
	err := s.table.getLogData(c, rk, 0, true, func(rk *rowKey, data []byte) error {
		latest = rk
		return nil
	})
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    s.Project,
			"zone":       s.Zone,
			"cluster":    s.Cluster,
			"table":      s.LogTable,
		}.Errorf(c, "Failed to scan for tail.")
	}

	if latest == nil {
		// No rows for the specified stream.
		return nil, 0, storage.ErrDoesNotExist
	}

	// Fetch the latest row's data.
	var d []byte
	err = s.table.getLogData(c, latest, 1, false, func(rk *rowKey, data []byte) error {
		d = data
		return errStop
	})
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    s.Project,
			"zone":       s.Zone,
			"cluster":    s.Cluster,
			"table":      s.LogTable,
		}.Errorf(c, "Failed to retrieve tail row.")
	}

	return d, types.MessageIndex(latest.index), nil
}

func (s *btStorage) getClient() (*bigtable.Client, error) {
	if s.client == nil {
		var err error
		if s.client, err = bigtable.NewClient(s.ctx, s.Project, s.Zone, s.Cluster, s.ClientOptions...); err != nil {
			return nil, fmt.Errorf("failed to create client: %s", err)
		}
	}
	return s.client, nil
}

func (s *btStorage) getAdminClient() (*bigtable.AdminClient, error) {
	if s.adminClient == nil {
		var err error
		if s.adminClient, err = bigtable.NewAdminClient(s.ctx, s.Project, s.Zone, s.Cluster, s.ClientOptions...); err != nil {
			return nil, fmt.Errorf("failed to create client: %s", err)
		}
	}
	return s.adminClient, nil
}

// getLogTable returns a btTable instance. If one is not already configured, a
// production instance will be generated and cached.
func (s *btStorage) getLogTable() (*bigtable.Table, error) {
	if s.logTable == nil {
		client, err := s.getClient()
		if err != nil {
			return nil, err
		}
		s.logTable = client.Open(s.LogTable)
	}
	return s.logTable, nil
}
