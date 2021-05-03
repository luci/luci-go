// Copyright 2021 The LUCI Authors.
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

package service

import (
	"context"
	"flag"

	cloudBT "cloud.google.com/go/bigtable"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/storage/bigtable"
	"go.chromium.org/luci/server/auth"
)

// storageFlags contains the intermediate storage config.
type storageFlags struct {
	// Project is the name of the Cloud Project containing the BigTable instance.
	Project string
	// Instance if the name of the BigTable instance within the project.
	Instance string
	// LogTable is the name of the BigTable instance's log table.
	LogTable string
}

// register registers flags in the flag set.
func (f *storageFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.Project, "bigtable-project", f.Project,
		"Cloud Project containing the BigTable instance.")
	fs.StringVar(&f.Instance, "bigtable-instance", f.Instance,
		"BigTable instance within the project to use.")
	fs.StringVar(&f.LogTable, "bigtable-log-table", f.LogTable,
		"Name of the table with logs.")
}

// validate returns an error if some parsed flags have invalid values.
func (f *storageFlags) validate() error {
	if f.Project == "" {
		return errors.New("-bigtable-project is required")
	}
	if f.Instance == "" {
		return errors.New("-bigtable-instance is required")
	}
	if f.LogTable == "" {
		return errors.New("-bigtable-log-table is required.")
	}
	return nil
}

// intermediateStorage instantiates the intermediate Storage instance.
func intermediateStorage(ctx context.Context, f *storageFlags) (storage.Storage, error) {
	ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get the token source").Err()
	}
	client, err := cloudBT.NewClient(ctx, f.Project, f.Instance, option.WithTokenSource(ts))
	if err != nil {
		return nil, errors.Annotate(err, "failed to construct BigTable client").Err()
	}
	return &bigtable.Storage{
		Client:   client,
		LogTable: f.LogTable,
	}, nil
}
