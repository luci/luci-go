// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"fmt"

	"github.com/luci/luci-go/common/errors"
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

	// EnableGarbageCollection is used during initialization only. If true, it
	// will enable pushing garbage collection settings to BigTable.
	//
	// At the time of writing, attempts to use garbage collection configuration
	// methods result in an error from the server stating that the methods are not
	// yet available.
	//
	// TODO(dnj): Remove this and default to true as soon as garbage collection
	// is enabled.
	EnableGarbageCollection bool
}

// btStorage is a storage.Storage implementation that uses Google Cloud BigTable
// as a backend.
type btStorage struct {
	*Options

	ctx    context.Context
	client *bigtable.Client

	table btTable
}

// New instantiates a new Storage instance connected to a BigTable cluster.
//
// The returned Storage instance will close the Client when its Close() method
// is called.
func New(ctx context.Context, o Options) (storage.Storage, error) {
	c, err := bigtable.NewClient(ctx, o.Project, o.Zone, o.Cluster, o.ClientOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %s", err)
	}

	return &btStorage{
		Options: &o,
		ctx:     ctx,
		client:  c,
		table:   &btTableProd{c.Open(o.LogTable)},
	}, nil
}

func (s *btStorage) Close() {
	s.client.Close()
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

// Purge iterates over each key sharing a stream prefix and deletes it.
//
// We do this by iterating over all rows that share the key, then deleting them.
// Finally, we will re-iterate over all rows that share the key and return an
// error if any still exist.
func (s *btStorage) Purge(p types.StreamPath) error {
	c := log.SetField(s.ctx, "path", p)

	// Listen for rows to purge. If any errors are encountered, aggregate them
	// in a MultiError for handling.
	rowC := make(chan *rowKey)
	errC := make(chan errors.MultiError)

	go func() {
		var deleteErr errors.MultiError
		defer func() {
			errC <- deleteErr
			close(errC)
		}()

		count := 0
		for rk := range rowC {
			if err := s.table.deleteRow(c, rk); err != nil {
				deleteErr = append(deleteErr, err)
			} else {
				count++
			}
		}

		log.Fields{
			"purgedRowCount": count,
		}.Debugf(c, "Purged rows.")
	}()

	// Run a keys-only query through the table.
	rk := newRowKey(string(p), 0)
	func() {
		defer close(rowC)

		s.table.getLogData(c, rk, 0, true, func(rk *rowKey, data []byte) error {
			rowC <- rk
			return nil
		})
	}()

	merr := <-errC
	if len(merr) > 0 {
		log.Fields{
			log.ErrorKey: merr,
			"errorCount": len(merr),
		}.Errorf(c, "Failed to purge log stream.")

		// If any of our sub-errors were transient, report the larger error as
		// transient.
		for _, e := range merr {
			if errors.IsTransient(e) {
				return errors.WrapTransient(merr)
			}
		}
		return merr
	}

	// Assert that there is no more data.
	return s.table.getLogData(c, rk, 0, true, func(rk *rowKey, data []byte) error {
		log.Fields{
			"rowKey": rk,
		}.Errorf(c, "Encountered row data post-purge.")
		return errors.New("encountered row data post-purge")
	})
}
