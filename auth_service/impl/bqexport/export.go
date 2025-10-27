// Copyright 2025 The LUCI Authors.
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

// Package bqexport contains functionality related to exporting the
// authorization data from LUCI Auth Service to BigQuery (BQ).
package bqexport

import (
	"context"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/permissionscfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/settingscfg"
	"go.chromium.org/luci/auth_service/internal/realmsinternals"
)

func CronHandler(ctx context.Context) error {
	logging.Infof(ctx, "attempting BQ export")

	if err := Run(ctx); err != nil {
		err = errors.Fmt("failed BQ export: %w", err)
		logging.Errorf(ctx, err.Error())
		return err
	}

	return nil
}

// Run exports the authorization data from the latest AuthDB snapshot to BQ.
func Run(ctx context.Context) error {
	// Ensure BQ export has been enabled before continuing.
	cfg, _, err := settingscfg.Get(ctx)
	if err != nil {
		return errors.Fmt("error getting settings.cfg: %w", err)
	}
	if !cfg.EnableBqExport {
		logging.Infof(ctx, "BQ export is disabled")
		return nil
	}

	start := timestamppb.New(clock.Now(ctx))
	latest, err := model.GetAuthDBSnapshotLatest(ctx)
	if err != nil {
		return errors.Fmt("failed to get latest snapshot: %w", err)
	}

	authDB, err := model.GetAuthDBFromSnapshot(ctx, latest.AuthDBRev)
	if err != nil {
		return errors.Fmt("failed to parse AuthDB from latest snapshot: %w", err)
	}

	return doExport(ctx, authDB, latest.AuthDBRev, start)
}

func doExport(ctx context.Context, authDB *protocol.AuthDB,
	authDBRev int64, ts *timestamppb.Timestamp) (reterr error) {
	start := clock.Now(ctx)
	groupRows, err := expandGroups(ctx, authDB, authDBRev, ts)
	logging.Debugf(ctx, "expanding groups took %s", clock.Since(ctx, start))
	if err != nil {
		return errors.Fmt("failed to expand all groups: %w", err)
	}

	start = clock.Now(ctx)
	realmRows, err := parseRealms(ctx, authDB, authDBRev, ts)
	logging.Debugf(ctx, "parsing realms took %s", clock.Since(ctx, start))
	if err != nil {
		return errors.Fmt("failed to make realm rows for export: %w", err)
	}

	client, err := NewClient(ctx)
	if err != nil {
		return err
	}
	defer func() {
		err := client.Close()
		if reterr == nil {
			reterr = errors.WrapIf(err, "failed to close BQ client")
		}
	}()

	// Insert all groups.
	start = clock.Now(ctx)
	err = client.InsertGroups(ctx, groupRows)
	logging.Debugf(ctx, "inserting BQ rows for groups took %s", clock.Since(ctx, start))
	if err != nil {
		return errors.Fmt("failed to insert all groups for AuthDB rev %d at %s: %w",
			authDBRev, ts.String(), err)
	}

	// Insert all realms.
	start = clock.Now(ctx)
	err = client.InsertRealms(ctx, realmRows)
	logging.Debugf(ctx, "inserting BQ rows for realms took %s", clock.Since(ctx, start))
	if err != nil {
		return errors.Fmt("failed to insert all realms for AuthDB rev %d at %s: %w",
			authDBRev, ts.String(), err)
	}

	if err := exportSupplementalData(ctx, client, ts); err != nil {
		// Non-fatal; just log the error.
		logging.Warningf(ctx, "failed when handling supplemental data: %s", err)
	}

	// Ensure the views for the latest data, to propagate schema changes.
	start = clock.Now(ctx)
	err = client.EnsureLatestViews(ctx)
	logging.Debugf(ctx, "ensuring latest views took %s", clock.Since(ctx, start))
	if err != nil {
		return err
	}

	return nil
}

type AuthConfig interface {
	*configspb.PermissionsConfig | *realmsconf.RealmsCfg
}

type ViewableConfig[T AuthConfig] struct {
	ViewURL string
	Config  T
}

type LatestConfigs struct {
	Permissions *ViewableConfig[*configspb.PermissionsConfig]
	Realms      map[string]*ViewableConfig[*realmsconf.RealmsCfg]
}

func getLatestConfigs(ctx context.Context) (*LatestConfigs, error) {
	permsCfg, permsMeta, err := permissionscfg.Get(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to get permissions.cfg: %w", err)
	}
	result := &LatestConfigs{
		Permissions: &ViewableConfig[*configspb.PermissionsConfig]{
			ViewURL: permsMeta.ViewURL,
			Config:  permsCfg,
		},
	}

	latest, err := realmsinternals.FetchLatestRealmsConfigs(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to fetch latest realms: %w", err)
	}
	result.Realms = make(map[string]*ViewableConfig[*realmsconf.RealmsCfg], len(latest))
	for k, v := range latest {
		parsed := &realmsconf.RealmsCfg{}
		if err := prototext.Unmarshal([]byte(v.Content), parsed); err != nil {
			return nil, errors.Fmt("failed to unmarshal config body for realms: %w", err)
		}
		result.Realms[k] = &ViewableConfig[*realmsconf.RealmsCfg]{
			ViewURL: v.ViewURL,
			Config:  parsed,
		}
	}

	return result, nil
}

func exportSupplementalData(ctx context.Context, client *Client,
	ts *timestamppb.Timestamp) error {
	start := clock.Now(ctx)
	latest, err := getLatestConfigs(ctx)
	logging.Debugf(ctx, "getting latest configs took %s", clock.Since(ctx, start))
	if err != nil {
		return err
	}

	// Attempt writing both latest roles and latest realms.

	start = clock.Now(ctx)
	roles := collateLatestRoles(ctx, latest, ts)
	logging.Debugf(ctx, "collating roles took %s", clock.Since(ctx, start))
	start = clock.Now(ctx)
	rolesErr := client.InsertRoles(ctx, roles)
	logging.Debugf(ctx, "inserting BQ rows for roles took %s", clock.Since(ctx, start))

	start = clock.Now(ctx)
	realmSources, realmsErr := expandLatestRealms(ctx, latest.Realms, ts)
	logging.Debugf(ctx, "expanding latest realms took %s", clock.Since(ctx, start))
	if realmsErr == nil {
		start = clock.Now(ctx)
		realmsErr = client.InsertRealmSources(ctx, realmSources)
		logging.Debugf(ctx, "inserting BQ rows for realm sources took %s", clock.Since(ctx, start))
	}

	// Return the first error that occurred.
	if rolesErr != nil {
		return errors.Fmt("failed to insert all %d roles: %w", len(roles), rolesErr)
	}
	if realmsErr != nil {
		return errors.Fmt("failed handling latest realms: %w", err)
	}

	return nil
}
