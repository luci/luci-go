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
	"time"

	"cloud.google.com/go/bigquery"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/storage"
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
	p        Parameters
	client   *bigquery.Client
	uploader *bigquery.Uploader
}

// NewSink creates a new Sink instance.
//
// It uses default application credentials to access BigQuery.
func NewSink(ctx context.Context, p Parameters) (*Sink, error) {
	bq, err := bigquery.NewClient(ctx, p.ProjectID)
	if err != nil {
		return nil, err
	}
	u := bq.DatasetInProject(p.ProjectID, p.DatasetID).Table(p.TableID).Uploader()
	return &Sink{p, bq, u}, nil
}

// StorageID is returned as storage.WriteStorage ID.
func (s *Sink) StorageID() string {
	return fmt.Sprintf("bq:%s:%s.%s", s.p.ProjectID, s.p.DatasetID, s.p.TableID)
}

// Close releases the allocated resources.
func (s *Sink) Close() {
	s.client.Close()
}

// put transforms the raw logs into BigQuery rows and uploads them.
func (s *Sink) put(ctx context.Context, desc *logpb.LogStreamDescriptor, r *storage.PutRequest) error {
	// BigQuery library retries transient uploads until context deadline.
	ctx, cancel := clock.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// TODO(vadimsh): Make the transformation configurable.
	rows := logsToRows(desc, r)

	// Ignore errors for now.
	logging.Infof(ctx, "Uploading %d row to %q", len(rows), s.StorageID())
	err := s.uploader.Put(ctx, toStuctSavers(rows))
	if merr, _ := err.(bigquery.PutMultiError); len(merr) != 0 {
		for _, err := range merr {
			logging.Errorf(ctx, "Failed to insert row into BigQuery: %s", err)
		}
	}
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
	return s.Sink.put(ctx, s.Desc, &r)
}
