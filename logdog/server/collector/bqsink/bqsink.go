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

// New creates new BigQuery write-only storage with given parameters.
func New(p Parameters) storage.WriteStorage {
	return &storageImpl{p}
}

// storageImpl implements storage.WriteStorage on top of BigQuery.
type storageImpl struct {
	p Parameters
}

func (s *storageImpl) StorageID() string {
	return fmt.Sprintf("bq:%s:%s.%s", s.p.ProjectID, s.p.DatasetID, s.p.TableID)
}

func (s *storageImpl) Close() {
	// nothing here for now
}

func (s *storageImpl) Config(ctx context.Context, cfg storage.Config) error {
	return nil // nothing here for now
}

func (s *storageImpl) Put(ctx context.Context, r storage.PutRequest) error {
	// TODO(vadimsh): Extract the data from 'r', transform it into a bunch of
	// BigQuery rows and insert them.
	return nil
}
