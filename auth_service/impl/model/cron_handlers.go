// Copyright 2024 The LUCI Authors.
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

package model

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/allowlistcfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/importscfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/oauthcfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/permissionscfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/securitycfg"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/settingscfg"
	"go.chromium.org/luci/auth_service/internal/configs/validation"
	"go.chromium.org/luci/auth_service/internal/permissions"
	"go.chromium.org/luci/auth_service/internal/pubsub"
	"go.chromium.org/luci/auth_service/internal/realmsinternals"
)

const (
	// The maximum age of the AuthDB where it is considered fresh.
	maxAuthDBAge = 24 * time.Hour

	// The maximum number of AuthDB revisions to produce when permissions
	// change and realms need to be reevaluated.
	maxReevaluationRevisions int = 10
)

// Set of config paths for configs that affect the AuthDB.
var authDBConfigPaths = stringset.NewFromSlice("ip_allowlist.cfg", "oauth.cfg", "security.cfg")

//////////////// Handling of stale replicated AuthDB //////////////////////

// ReplicatedAuthDBRefresher triggers AuthDB replication if it hasn't been done
// recently, as defined by maxAuthDBAge.
//
// Called periodically as a cron job. If it detects that the last AuthDB
// revision was produced awhile ago, bumps AuthDB revision number and
// triggers replication (actual contents of AuthDB is not changed).
//
// This is important to make sure the AuthDB replication configuration doesn't
// rot and that the exported AuthDB blob has a relatively fresh signature.
func ReplicatedAuthDBRefresher(ctx context.Context) error {
	replicationState, err := GetReplicationState(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to get current replication state").Err()
	}

	age := clock.Now(ctx).UTC().Sub(replicationState.ModifiedTS)
	if age < maxAuthDBAge {
		logging.Infof(ctx, "replicated AuthDB is fresh: %s < %s",
			age, maxAuthDBAge)
		return nil
	}

	logging.Warningf(ctx, "refreshing replicated AuthDB: %s > %s",
		age, maxAuthDBAge)

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Get current AuthDB state and increment the revision.
		state, err := GetReplicationState(ctx)
		if err != nil {
			return err
		}

		if state.AuthDBRev != replicationState.AuthDBRev {
			// Nothing to do - the AuthDB was updated while we were checking
			// its age.
			return nil
		}

		// Increment the AuthDB revision and update the modified timestamp.
		state.AuthDBRev = state.AuthDBRev + 1
		state.ModifiedTS = clock.Now(ctx).UTC()

		// Commit the updated replication state.
		if err := datastore.Put(ctx, state); err != nil {
			return err
		}

		// Enqueue two backend tasks:
		// - one to generate the changelog (which should be empty but now this
		//   revision will have a record its changes were processed); and
		// - one to replicate the updated AuthDB to clients.
		if err := EnqueueProcessChangeTask(ctx, state.AuthDBRev); err != nil {
			return err
		}
		return EnqueueReplicationTask(ctx, state.AuthDBRev)
	}, nil)
	if err != nil {
		logging.Errorf(ctx, "failed to refresh stale AuthDB: %s", err)
		return err
	}

	return nil
}

//////////////////// Handling of stale authorizations //////////////////////////

func StaleAuthorizationCronHandler(ctx context.Context) error {
	// Only members of the below trusted group are eligible to:
	// * be authorized to subscribe to PubSub notifications of AuthDB changes
	// * be authorized to read the AuthDB from Google Storage.
	// This cron revokes all stale authorizations for accounts that are no
	// longer in the trusted group.
	trustedGroup := TrustedServicesGroup

	if err := pubsub.RevokeStaleAuthorization(ctx, trustedGroup); err != nil {
		err = errors.Annotate(err, "error revoking stale PubSub authorizations").Err()
		logging.Errorf(ctx, err.Error())
		return err
	}

	if err := RevokeStaleReaderAccess(ctx, trustedGroup); err != nil {
		err = errors.Annotate(err, "error revoking stale AuthDB reader access").Err()
		logging.Errorf(ctx, err.Error())
		return err
	}

	return nil
}

/////////////////////// Handling of service configs ////////////////////////////

func ServiceConfigCronHandler(ctx context.Context) error {
	historicalComment := "Updated from update-config cron"

	if err := refreshServiceConfigs(ctx); err != nil {
		return err
	}

	if err := applyGlobalConfigUpdate(ctx, historicalComment); err != nil {
		return err
	}

	if err := applyAllowlistUpdate(ctx, historicalComment); err != nil {
		return err
	}

	return nil
}

type configRefresher func(ctx context.Context) (*config.Meta, error)

// refreshServiceConfigs updates the cached service configs to be the latest
// from LUCI Config.
func refreshServiceConfigs(ctx context.Context) error {
	configRefreshers := []configRefresher{
		allowlistcfg.Update,
		importscfg.Update,
		oauthcfg.Update,
		securitycfg.Update,
		settingscfg.Update,
	}

	eg, childCtx := errgroup.WithContext(ctx)
	for _, refresher := range configRefreshers {
		eg.Go(func() error {
			if _, err := refresher(childCtx); err != nil {
				// Log the error, so details aren't lost if there are multiple
				// errors.
				logging.Errorf(childCtx, err.Error())
				return err
			}

			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

// applyAllowlistUpdate applies the current ip_allowlist.cfg to all
// AuthIPAllowlist entities.
func applyAllowlistUpdate(ctx context.Context, historicalComment string) error {
	cfg, err := allowlistcfg.Get(ctx)
	if err != nil {
		return err
	}

	subnets, err := validation.GetSubnets(cfg.IpAllowlists)
	if err != nil {
		return err
	}

	if err := updateAllAuthIPAllowlists(ctx, subnets, historicalComment); err != nil {
		return err
	}

	return nil
}

// applyGlobalConfigUpdate applies the current oauth.cfg and security.cfg
// to the AuthGlobalConfig entity.
func applyGlobalConfigUpdate(ctx context.Context, historicalComment string) error {
	oauthConfig, err := oauthcfg.Get(ctx)
	if err != nil {
		return err
	}
	securityConfig, err := securitycfg.Get(ctx)
	if err != nil {
		return err
	}

	if err := updateAuthGlobalConfig(ctx, oauthConfig, securityConfig, historicalComment); err != nil {
		return err
	}

	return nil
}

/////////////////////// Handling of realms configs /////////////////////////////

func RealmsConfigCronHandler(ctx context.Context) error {
	historicalComment := "Updated from update-realms cron"

	// permissions.cfg handling.
	if err := permissionscfg.Update(ctx); err != nil {
		return err
	}
	permsCfg, permsMeta, err := permissionscfg.GetWithMetadata(ctx)
	if err != nil {
		return err
	}
	if err := updateAuthRealmsGlobals(ctx, permsCfg, historicalComment); err != nil {
		return err
	}

	// Make the PermissionsDB for realms expansion.
	permsDB := permissions.NewPermissionsDB(permsCfg, permsMeta)

	// realms.cfg handling.
	latestRealms, err := getLatestRealmsCfgRev(ctx)
	if err != nil {
		logging.Errorf(ctx, "aborting realms update - failed to fetch latest for all configs: %v", err)
		return err
	}
	storedRealms, err := getStoredRealmsCfgRevs(ctx)
	if err != nil {
		logging.Errorf(ctx, "aborting realms update - failed to get stored configs: %v", err)
		return err
	}

	jobs, err := processRealmsConfigChanges(ctx, permsDB, latestRealms, storedRealms, historicalComment)
	if err != nil {
		return err
	}
	if !executeJobs(ctx, jobs, 2*time.Second) {
		return fmt.Errorf("not all jobs succeeded when refreshing realms")
	}

	return nil
}

// processRealmsConfigChanges returns a slice of parameterless callbacks to
// update the AuthDB based on detected realms.cfg and permissions
// changes.
//
// Args:
//   - permissionsDB: the current permissions and roles;
//   - latest: RealmsCfgRev's for the realms configs fetched from
//     LUCI Config;
//   - stored: RealmsCfgRev's for the last processed realms configs;
//   - historicalComment: the comment to use in entities' history if
//     changes are committed.
//
// Returns:
//   - jobs: parameterless callbacks to update the AuthDB.
func processRealmsConfigChanges(
	ctx context.Context, permissionsDB *permissions.PermissionsDB,
	latest []*RealmsCfgRev, stored []*RealmsCfgRev,
	historicalComment string) ([]func() error, error) {
	toMap := func(revisions []*RealmsCfgRev) (map[string]*RealmsCfgRev, error) {
		result := make(map[string]*RealmsCfgRev, len(revisions))
		for _, cfgRev := range revisions {
			result[cfgRev.ProjectID] = cfgRev
		}

		if len(result) != len(revisions) {
			return nil, fmt.Errorf("multiple realms configs for the same project ID")
		}
		return result, nil
	}

	latestMap, err := toMap(latest)
	if err != nil {
		return nil, err
	}
	storedMap, err := toMap(stored)
	if err != nil {
		return nil, err
	}

	var jobs []func() error

	// For the realms configs that should be reevaluated, because they
	// were generated with a previous revision of permissions.
	toReevaluate := []*RealmsCfgRev{}

	// Detect changes to realms configs. Going through the latest
	// configs in a random order helps to progress if one of the configs
	// is somehow very problematic (e.g. causes OOM). When the cron job
	// is repeatedly retried, all healthy configs will eventually be
	// processed before the problematic ones.
	randomOrder := mathrand.Perm(ctx, len(latest))
	for _, i := range randomOrder {
		latestCfgRev := latest[i]
		storedCfgRev, ok := storedMap[latestCfgRev.ProjectID]
		if !ok || (storedCfgRev.ConfigDigest != latestCfgRev.ConfigDigest) {
			// Add a job to update this project's realms.
			revs := []*RealmsCfgRev{latestCfgRev}
			comment := fmt.Sprintf("%s - using realms config rev %s", historicalComment, latestCfgRev.ConfigRev)
			jobs = append(jobs, func() error {
				return updateRealms(ctx, permissionsDB, revs, comment)
			})
		} else if storedCfgRev.PermsRev != permissionsDB.Rev {
			// This config needs to be reevaluated.
			toReevaluate = append(toReevaluate, latestCfgRev)
		}
	}

	// Detect realms.cfg that were removed completely.
	for _, storedCfgRev := range stored {
		if _, ok := latestMap[storedCfgRev.ProjectID]; !ok {
			// Add a job to delete this project's realms.
			projID := storedCfgRev.ProjectID
			comment := fmt.Sprintf("%s - config no longer exists", historicalComment)
			jobs = append(jobs, func() error {
				return deleteRealms(ctx, projID, comment)
			})
		}
	}

	// Changing the permissions (e.g. adding a new permission to a widely used
	// role) may affect ALL projects. In this case, generating a ton of AuthDB
	// revisions is wasteful. We could try to generate a single giant revision,
	// but it may end up being too big, hitting datastore limits. So we
	// "heuristically" split it into at most maxReevaluationRevisions, hoping
	// for the best.
	reevaluations := len(toReevaluate)
	batchSize := reevaluations / maxReevaluationRevisions
	if batchSize < 1 {
		batchSize = 1
	}
	for i := 0; i < reevaluations; i += batchSize {
		j := i + batchSize
		if j > reevaluations {
			j = reevaluations
		}
		revs := toReevaluate[i:j]
		comment := fmt.Sprintf("%s - generating realms with permissions rev %s",
			historicalComment, permissionsDB.Rev)
		jobs = append(jobs, func() error {
			return updateRealms(ctx, permissionsDB, revs, comment)
		})
	}

	return jobs, nil
}

func getStoredRealmsCfgRevs(ctx context.Context) ([]*RealmsCfgRev, error) {
	// Get project realms config metadata in datastore.
	storedMeta, err := GetAllAuthProjectRealmsMeta(ctx)
	if err != nil {
		return nil, err
	}

	// Log the projects that have stored AuthProjectRealmsMeta, to aid in
	// debugging.
	projectsWithMeta := make([]string, len(storedMeta))
	for i, meta := range storedMeta {
		metaProj, _ := meta.ProjectID()
		projectsWithMeta[i] = metaProj
	}
	logging.Debugf(ctx, "fetched realms config metadata for %d projects: %s",
		len(storedMeta), projectsWithMeta)

	storedRevs := make([]*RealmsCfgRev, len(storedMeta))
	for i, meta := range storedMeta {
		projID, err := meta.ProjectID()
		if err != nil {
			return nil, err
		}
		storedRevs[i] = &RealmsCfgRev{
			ProjectID:    projID,
			ConfigRev:    meta.ConfigRev,
			ConfigDigest: meta.ConfigDigest,
			PermsRev:     meta.PermsRev,
		}
	}
	return storedRevs, nil
}

func getLatestRealmsCfgRev(ctx context.Context) ([]*RealmsCfgRev, error) {
	latestConfigs, err := realmsinternals.FetchLatestRealmsConfigs(ctx)
	if err != nil {
		return nil, err
	}

	latestRevs := make([]*RealmsCfgRev, len(latestConfigs))
	idx := 0
	for projID, cfg := range latestConfigs {
		latestRevs[idx] = &RealmsCfgRev{
			ProjectID:    projID,
			ConfigRev:    cfg.Revision,
			ConfigDigest: cfg.ContentHash,
			ConfigBody:   []byte(cfg.Content),
		}
		idx++
	}

	return latestRevs, nil
}

// updateRealms updates the project realms for each of the realms configs given.
//
// Returns an annotated error if one occurred, such as:
// - failed to unmarshal to a proto;
// - failed to expand realms; or
// - failed to update datastore with realms changes.
func updateRealms(ctx context.Context, db *permissions.PermissionsDB, revs []*RealmsCfgRev, historicalComment string) error {
	expanded := []*ExpandedRealms{}
	for _, r := range revs {
		logging.Infof(ctx, "expanding realms of project \"%s\"...", r.ProjectID)
		start := time.Now()

		parsed := &realmsconf.RealmsCfg{}
		if err := prototext.Unmarshal(r.ConfigBody, parsed); err != nil {
			return errors.Annotate(err, "couldn't unmarshal config body").Err()
		}
		expandedRev, err := realmsinternals.ExpandRealms(ctx, db, r.ProjectID, parsed)
		if err != nil {
			return errors.Annotate(err, "failed to process realms of \"%s\"", r.ProjectID).Err()
		}
		expanded = append(expanded, &ExpandedRealms{
			CfgRev: r,
			Realms: expandedRev,
		})

		if dt := time.Since(start).Seconds(); dt > 5.0 {
			logging.Warningf(ctx, "realms expansion of \"%s\" is slow: %1.f seconds", r.ProjectID, dt)
		}
	}
	if len(expanded) == 0 {
		return nil
	}

	logging.Infof(ctx, "entering transaction")
	if err := updateAuthProjectRealms(ctx, expanded, db.Rev, historicalComment); err != nil {
		return err
	}
	logging.Infof(ctx, "transaction landed")
	return nil
}

// deleteRealms will try to delete the AuthProjectRealms & AuthProjectRealmsMeta
// for a given projectID.
func deleteRealms(ctx context.Context, projectID string, historicalComment string) error {
	switch err := deleteAuthProjectRealms(ctx, projectID, historicalComment); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		logging.Debugf(ctx, "realms for %s do not exist or have already been deleted", projectID)

		// Attempt to delete the corresponding AuthProjectRealmsMeta, as an
		// extra clean-up step. This ensures the meta realms mirror the
		// project realms, in case deleting meta realms failed previously.
		metaErr := deleteAuthProjectRealmsMeta(ctx, projectID)
		if metaErr != nil && !errors.Is(metaErr, datastore.ErrNoSuchEntity) {
			return metaErr
		}
		return nil
	case err != nil:
		return err
	default:
		logging.Infof(ctx, "deleted realms for %s", projectID)
		return nil
	}
}

// executeJobs executes the callbacks, sleeping the set amount of time
// between each. Note: all callbacks will be run, even if a previous job
// returned an error.
//
// Returns whether any job returned an error.
func executeJobs(ctx context.Context, jobs []func() error, sleepTime time.Duration) bool {
	success := true
	for i, job := range jobs {
		if i > 0 {
			time.Sleep(sleepTime)
		}
		if err := job(); err != nil {
			logging.Errorf(ctx, "job %d out of %d failed: %s", i+1, len(jobs), err)
			success = false
		}
	}
	return success
}
