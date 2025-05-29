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

package bigtable

import (
	"context"
	"flag"

	"cloud.google.com/go/bigtable"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
)

// Flags contains the BigTable storage config.
type Flags struct {
	// Project is the name of the Cloud Project containing the BigTable instance.
	Project string
	// Instance if the name of the BigTable instance within the project.
	Instance string
	// LogTable is the name of the BigTable instance's log table.
	LogTable string

	// AppProfile is the BigTable application profile name to use (or "" for
	// default).
	//
	// This is INTENTIONALLY not wired to a CLI flag; The value here is tied to
	// the _code_, not the runtime environment of the code.
	//
	// The application profile must be configured in the GCP BigTable settings
	// before use.
	//
	// However, in the future it may become necessary to disambiguate between e.g.
	// prod and dev. If this is the case, then I would recommend StorageFromFlags
	// adding "-prod" and "-dev" to the given AppProfile name here, rather than
	// making it fully configurable as a CLI flag (to reduce coupling during
	// rollouts).
	AppProfile string
}

// Register registers flags in the flag set.
func (f *Flags) Register(fs *flag.FlagSet) {
	fs.StringVar(&f.Project, "bigtable-project", f.Project,
		"Cloud Project containing the BigTable instance.")
	fs.StringVar(&f.Instance, "bigtable-instance", f.Instance,
		"BigTable instance within the project to use.")
	fs.StringVar(&f.LogTable, "bigtable-log-table", f.LogTable,
		"Name of the table with logs.")
}

// Validate returns an error if some parsed flags have invalid values.
func (f *Flags) Validate() error {
	if f.Project == "" {
		return errors.New("-bigtable-project is required")
	}
	if f.Instance == "" {
		return errors.New("-bigtable-instance is required")
	}
	if f.LogTable == "" {
		return errors.New("-bigtable-log-table is required")
	}
	return nil
}

// StorageFromFlags instantiates the *bigtable.Storage given parsed flags.
func StorageFromFlags(ctx context.Context, f *Flags) (*Storage, error) {
	ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Fmt("failed to get the token source: %w", err)
	}
	cCfg := bigtable.ClientConfig{
		AppProfile: f.AppProfile,
	}
	client, err := bigtable.NewClientWithConfig(ctx, f.Project, f.Instance, cCfg, option.WithTokenSource(ts))
	if err != nil {
		return nil, errors.Fmt("failed to construct BigTable client: %w", err)
	}
	return &Storage{
		Client:   client,
		LogTable: f.LogTable,
	}, nil
}
