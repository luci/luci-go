// Copyright 2017 The LUCI Authors.
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

// bqsink contains definition of storage.WriteStorage that uploads logs to
// BigQuery.
package bqsink

import (
	"fmt"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/storage"

	"golang.org/x/net/context"
)

// Parameters specify parameters for the sink.
//
// Note that it will be used as a map key, so use only simple types for fields
// here.
type Parameters struct {
	ProjectID string
	DatasetID string
	TableID   string
}

// Sink is long-living object that knows how to upload log data to BigQuery.
//
// It may be shared by multiple goroutines and should implement proper
// synchronization itself.
type Sink struct {
	p Parameters
}

// NewSink creates a new Sink instance.
func NewSink(p Parameters) *Sink {
	return &Sink{p}
}

// storageID is returned as storage.WriteStorage ID.
func (s *Sink) StorageID() string {
	return fmt.Sprintf("bq:%s:%s.%s", s.p.ProjectID, s.p.DatasetID, s.p.TableID)
}

// put transforms the raw logs into BigQuery rows and uploads them.
func (s *Sink) put(ctx context.Context, desc *logpb.LogStreamDescriptor, r *storage.PutRequest) error {
	// TODO(vadimsh): Extract the data from 'r', transform it into a bunch of
	// BigQuery rows and insert them.
	return nil
}

// Storage is short-living storage.WriteStorage implementation that uploads
// logs of the specific log stream to the given Sink.
type Storage struct {
	Sink *Sink
	Desc *logpb.LogStreamDescriptor
}

// StorageID implements storage.WriteStorage.
func (s *Storage) StorageID() string {
	return s.Sink.StorageID()
}

// Close implements storage.WriteStorage.
func (s *Storage) Close() {
	// nothing here for now
}

// Config implements storage.WriteStorage.
func (s *Storage) Config(ctx context.Context, cfg storage.Config) error {
	return nil // nothing here for now
}

// Put implements storage.WriteStorage.
func (s *Storage) Put(ctx context.Context, r storage.PutRequest) error {
	// TODO(vadimsh): Extract the data from 'r', transform it into a bunch of
	// BigQuery rows and insert them.
	return nil
}
