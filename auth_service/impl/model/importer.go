// Copyright 2022 The LUCI Authors.
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
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/importscfg"
)

const (
	// Suffix for Google temporary accounts, which need to be handled
	// differently when constructing principal identities.
	gTempSuffix = "@gtempaccount.com"
)

var (
	// ErrConcurrentAuthDBUpdate signifies the AuthDB was modified in a concurrent transaction.
	ErrConcurrentAuthDBUpdate = errors.New("AuthDB changed between transactions")
	// ErrImporterNotConfigured is returned if there is no importer config.
	ErrImporterNotConfigured = errors.New("no importer config")
	// ErrInvalidTarball is returned if the tarball data is invalid.
	ErrInvalidTarball = errors.New("invalid tarball data")
	// ErrInvalidTarballName is returned if the tarball name is invalid.
	ErrInvalidTarballName = errors.New("invalid tarball name")
	// ErrUnauthorizedUploader is returned if the caller is not authorized to upload a given tarball.
	ErrUnauthorizedUploader = errors.New("unauthorized tarball uploader")
)

// Imports groups from some external tar.gz bundle or plain text list.
// External URL should serve *.tar.gz file with the following file structure:
//   <external group system name>/<group name>:
//     userid
//     userid
//     ...

// For example ldap.tar.gz may look like:
//   ldap/trusted-users:
//     jane
//     joe
//     ...
//   ldap/all:
//     jane
//     joe
//     ...

// Each tarball may have groups from multiple external systems, but groups from
// some external system must not be split between multiple tarballs. When importer
// sees <external group system name>/* in a tarball, it modifies group list from
// that system on the server to match group list in the tarball _exactly_,
// including removal of groups that are on the server, but no longer present in
// the tarball.

// Plain list format should have one userid per line and can only describe a single
// group in a single system. Such groups will be added to 'external/*' groups
// namespace. Removing such group from importer config will remove it from
// service too.

// The service can also be configured to accept tarball uploads (instead of
// fetching them). Fetched and uploaded tarballs are handled in the exact same way,
// in particular all caveats related to external group system names apply.

// GroupImporterConfig is a singleton entity that contains the contents of the imports.cfg file.
type GroupImporterConfig struct {
	Kind string `gae:"$kind,GroupImporterConfig"`
	ID   string `gae:"$id,config"`

	// ConfigProto is the plaintext copy of the config found at imports.cfg.
	ConfigProto string `gae:"config_proto"`

	// ConfigRevision is revision version of the config found at imports.cfg.
	ConfigRevision []byte `gae:"config_revision"`

	// ModifiedBy is the email of the user who modified the cfg.
	ModifiedBy string `gae:"modified_by,noindex"`

	// ModifiedTS is the time when this entity was last modified.
	ModifiedTS time.Time `gae:"modified_ts,noindex"`
}

// GroupBundle is a map where k: groupName, v: list of identities belonging to group k.
type GroupBundle = map[string][]identity.Identity

// GetGroupImporterConfig fetches the GroupImporterConfig entity from the datastore.
//
//	Returns GroupImporterConfig entity if present.
//	Returns datastore.ErrNoSuchEntity if the entity is not present.
//	Returns annotated error for all other errors.
func GetGroupImporterConfig(ctx context.Context) (*GroupImporterConfig, error) {
	groupsCfg := &GroupImporterConfig{
		Kind: "GroupImporterConfig",
		ID:   "config",
	}

	switch err := datastore.Get(ctx, groupsCfg); {
	case err == nil:
		return groupsCfg, nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting GroupImporterConfig").Err()
	}
}

// updateGroupImporterConfig updates the GroupImporterConfig datastore entity.
// If there is no GroupImporterConfig entity present in the datastore, one will
// be created.
//
// TODO(b/302615672): Remove this once Auth Service has been fully migrated to
// Auth Service v2. In v2, the GroupImporterConfig entity is redundant; the
// config proto and revision metadata is all handled by the importscfg package.
func updateGroupImporterConfig(ctx context.Context, importsCfg *configspb.GroupImporterConfig, meta *config.Meta, dryRun bool) error {
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		storedCfg, err := GetGroupImporterConfig(ctx)
		if err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
			return err
		}
		if storedCfg == nil {
			storedCfg = &GroupImporterConfig{
				Kind: "GroupImporterConfig",
				ID:   "config",
			}
		}

		latestRev := configRevisionInfo{
			Revision: meta.Revision,
			ViewURL:  meta.ViewURL,
		}

		// First, check if an update is necessary.
		if storedCfg.ConfigRevision != nil {
			var storedRev configRevisionInfo
			if err := json.Unmarshal(storedCfg.ConfigRevision, &storedRev); err != nil {
				return errors.Annotate(err,
					"failed to unmarshal revision info for GroupImporterConfig").Err()
			}

			if storedRev == latestRev {
				// Skip update since GroupImporterConfig is already up to date.
				return nil
			}
		}

		configContent, err := prototext.Marshal(importsCfg)
		if err != nil {
			return errors.Annotate(err, "failed to marshal imports.cfg content").Err()
		}
		configRev, err := json.Marshal(latestRev)
		if err != nil {
			return errors.Annotate(err, "failed to marshal revision info for GroupImporterConfig").Err()
		}
		serviceIdentity, err := getServiceIdentity(ctx)
		if err != nil {
			return err
		}

		// Exit early if in dry run mode.
		if dryRun {
			return nil
		}

		storedCfg.ConfigProto = string(configContent)
		storedCfg.ConfigRevision = configRev
		storedCfg.ModifiedBy = string(serviceIdentity)
		storedCfg.ModifiedTS = clock.Now(ctx).UTC()
		return datastore.Put(ctx, storedCfg)
	}, nil)
	if err != nil {
		return errors.Annotate(err, "failed to update GroupImporterConfig").Err()
	}

	return nil
}

// IngestTarball handles upload of tarball's specified in 'tarball_upload' config entries.
// expected to be called in an auth context of the upload PUT request.
//
// returns
//
//	[]string - list of modified groups
//	int64 - authDBRevision
//	error
//		ErrImporterNotConfigured if no importer config
//		ErrUnauthorizedUploader if caller is not an authorized uploader
//		ErrInvalidTarballName if tarball name is empty
//		ErrInvalidTarballName if entry not found in tarball upload config
//		ErrInvalidTarball if bad tarball structure
//		error from importing the tarball, if one occurs
func IngestTarball(ctx context.Context, name string, content io.Reader) ([]string, int64, error) {
	// Check if the caller is authorized to upload this tarball.
	caller := auth.CurrentIdentity(ctx)
	email := caller.Email()
	authorized, err := importscfg.IsAuthorizedUploader(ctx, email, name)
	if err != nil {
		return nil, 0, ErrImporterNotConfigured
	}
	if !authorized {
		return nil, 0, fmt.Errorf("%w: %q", ErrUnauthorizedUploader, email)
	}

	if name == "" {
		// This should be impossible in practice, because importer config
		// validation mandates the tarball name being set. Thus, there would be
		// no such entry in the config and the caller should not have been
		// considered an authorized uploader.
		return nil, 0, fmt.Errorf("%w: empty", ErrInvalidTarballName)
	}

	importsConfig, err := importscfg.Get(ctx)
	if err != nil {
		return nil, 0, ErrImporterNotConfigured
	}

	// Make sure the tarball_upload entry is specified in the config.
	var entry *configspb.GroupImporterConfig_TarballUploadEntry
	for _, tbu := range importsConfig.GetTarballUpload() {
		if tbu.Name == name {
			entry = tbu
			break
		}
	}
	if entry == nil {
		// This should be impossible to reach because of the early authorized
		// uploader check.
		return nil, 0, fmt.Errorf("%w: not supported in importer config",
			ErrInvalidTarballName)
	}

	bundles, err := loadTarball(ctx, content, entry.GetDomain(), entry.GetSystems(), entry.GetGroups())
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %s", ErrInvalidTarball, err.Error())
	}

	return importBundles(ctx, bundles, caller, nil)
}

// loadTarball unzips tarball with groups and deserializes them.
func loadTarball(ctx context.Context, content io.Reader, domain string, systems, groups []string) (map[string]GroupBundle, error) {
	// map looks like: K: system, V: { K: groupName, V: []identities }
	bundles := make(map[string]GroupBundle)
	entries, err := extractTarArchive(content)
	if err != nil {
		return nil, err
	}

	// verify system/groupname and then parse blob if valid
	for filename, fileobj := range entries {
		chunks := strings.Split(filename, "/")
		if len(chunks) != 2 || !auth.IsValidGroupName(filename) {
			logging.Warningf(ctx, "Skipping file %s, not a valid name", filename)
			continue
		}
		if groups != nil && !contains(filename, groups) {
			continue
		}
		system := chunks[0]
		if !contains(system, systems) {
			logging.Warningf(ctx, "Skipping file %s, not allowed", filename)
			continue
		}
		identities, err := loadGroupFile(string(fileobj), domain)
		if err != nil {
			return nil, err
		}
		if _, ok := bundles[system]; !ok {
			bundles[system] = make(GroupBundle)
		}
		bundles[system][filename] = identities
	}
	return bundles, nil
}

func loadGroupFile(identities string, domain string) ([]identity.Identity, error) {
	memsSplit := strings.Split(identities, "\n")
	uniqueMembers := make(map[identity.Identity]struct{}, len(memsSplit))
	for _, uid := range memsSplit {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		ident := fmt.Sprintf("user:%s", uid)
		if domain != "" {
			ident = fmt.Sprintf("%s@%s", ident, domain)
		}

		// Handle gtemp accounts. These emails look like
		// "name%domain@gtempaccount.com". Convert them to "name@domain".
		// See https://support.google.com/a/answer/185186?hl=en
		if strings.HasSuffix(ident, gTempSuffix) {
			ident = strings.TrimSuffix(ident, gTempSuffix)
			ident = strings.ReplaceAll(ident, "%", "@")
		}

		emailIdent, err := identity.MakeIdentity(ident)
		if err != nil {
			return nil, err
		}
		uniqueMembers[emailIdent] = struct{}{}
	}

	members := make([]identity.Identity, 0, len(uniqueMembers))
	for mem := range uniqueMembers {
		members = append(members, mem)
	}
	slices.Sort(members)

	return members, nil
}

// importBundles imports given set of bundles all at once.
// A bundle is a map with groups that is the result of a processing of some tarball.
// A bundle specifies the desired state of all groups under some system, e.g.
// importBundles({'ldap': {}}, ...) will REMOVE all existing 'ldap/*' groups.
//
// Group names in the bundle are specified in their full prefixed form (with
// system name prefix). An example of expected 'bundles':
//
//	{
//	  'ldap': {
//			'ldap/group': [Identity(...), Identity(...)],
//	  },
//	}
//
// Args:
//
//	bundles: map system name -> GroupBundle
//	providedBy: auth.Identity to put in modifiedBy or createdBy fields.
//
// Returns:
//
//	(list of modified groups,
//	new AuthDB revision number or 0 if no changes,
//	error if issue with writing entities).
func importBundles(ctx context.Context, bundles map[string]GroupBundle, providedBy identity.Identity, testHook func()) ([]string, int64, error) {
	// Nothing to process.
	if len(bundles) == 0 {
		return []string{}, 0, nil
	}

	getAuthDBRevision := func(ctx context.Context) (int64, error) {
		state, err := GetReplicationState(ctx)
		switch {
		case errors.Is(err, datastore.ErrNoSuchEntity):
			return 0, nil
		case err != nil:
			return -1, err
		default:
			return state.AuthDBRev, nil
		}
	}

	// Fetches all existing groups and AuthDB revision number.
	groupsSnapshot := func(ctx context.Context) (gMap map[string]*AuthGroup, rev int64, err error) {
		err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			groups, err := GetAllAuthGroups(ctx)
			if err != nil {
				return err
			}
			gMap = make(map[string]*AuthGroup, len(groups))
			for _, g := range groups {
				gMap[g.ID] = g
			}
			rev, err = getAuthDBRevision(ctx)
			if err != nil {
				return errors.Annotate(err, "couldn't get AuthDBRev").Err()
			}
			return nil
		}, nil)
		return gMap, rev, err
	}

	// Transactionally puts and deletes a bunch of entities.
	applyImport := func(expectedRevision int64, entitiesToPut, entitiesToDelete []*AuthGroup, ts time.Time) error {
		// Runs in transaction.
		return runAuthDBChange(ctx, "Imported from group bundles", func(ctx context.Context, cae commitAuthEntity) error {
			rev, err := getAuthDBRevision(ctx)
			if err != nil {
				return err
			}

			// DB changed between transactions.
			if rev != expectedRevision {
				return fmt.Errorf("%w: expected rev %d but it was actually %d",
					ErrConcurrentAuthDBUpdate, expectedRevision, rev)
			}
			for _, e := range entitiesToPut {
				if err := cae(e, ts, providedBy, false); err != nil {
					return err
				}
			}

			for _, e := range entitiesToDelete {
				if err := cae(e, ts, providedBy, true); err != nil {
					return err
				}
			}
			return nil
		})
	}

	updatedGroups := stringset.New(0)
	revision := int64(0)
	loopCount := 0
	var groups map[string]*AuthGroup
	var err error

	// Try to apply the change in batches until it lands completely or deadline
	// happens. Split each batch update into two transactions (assuming AuthDB
	// changes infrequently) to avoid reading and writing too much stuff from
	// within a single transaction (and to avoid keeping the transaction open while
	// calculating the diff).
	for {
		// Use same timestamp everywhere to reflect that groups were imported
		// atomically within a single transaction.
		ts := clock.Now(ctx).UTC()
		loopCount += 1
		groups, revision, err = groupsSnapshot(ctx)
		if err != nil {
			return nil, revision, err
		}
		// For testing purposes only.
		if testHook != nil && loopCount == 2 {
			testHook()
		}
		entitiesToPut := []*AuthGroup{}
		entitiesToDel := []*AuthGroup{}
		for sys := range bundles {
			iGroups := bundles[sys]
			toPut, toDel := prepareImport(ctx, sys, groups, iGroups, providedBy, ts)
			entitiesToPut = append(entitiesToPut, toPut...)
			entitiesToDel = append(entitiesToDel, toDel...)
		}

		if len(entitiesToPut) == 0 && len(entitiesToDel) == 0 {
			logging.Infof(ctx, "nothing to do")
			break
		}

		// An `applyImport` transaction can touch at most 500 entities. Cap the
		// number of entities we create/delete to 200 in total since we attach a
		// historical entity to each entity. The rest will be updated on the
		// next cycle of the loop. This is safe to do since:
		//  * Imported groups are "leaf" groups (have no subgroups) and can be
		//    added in arbitrary order without worrying about referential
		//    integrity.
		//  * Deleted groups are guaranteed to be unreferenced by
		//    `prepareImport` and can be deleted in arbitrary order as well.
		truncated := false

		// Both these operations happen in the same transaction so we have
		// to trim it to make sure the total is <= 200.
		if len(entitiesToPut) > 200 {
			entitiesToPut = entitiesToPut[:200]
			entitiesToDel = nil
			truncated = true
		} else if len(entitiesToPut)+len(entitiesToDel) > 200 {
			entitiesToDel = entitiesToDel[:200-len(entitiesToPut)]
			truncated = true
		}

		// Log what we are about to do to help debugging transaction errors.
		logging.Infof(ctx, "Preparing AuthDB rev %d with %d puts and %d deletes:", revision+1, len(entitiesToPut), len(entitiesToDel))
		for _, e := range entitiesToPut {
			logging.Infof(ctx, "U %s", e.ID)
			updatedGroups.Add(e.ID)
		}
		for _, e := range entitiesToDel {
			logging.Infof(ctx, "D %s", e.ID)
			updatedGroups.Add(e.ID)
		}

		// Check the AdminGroup exists before attempting to apply the import,
		// because it is the owning group for all external groups.
		if err := checkGroupsExist(ctx, []string{AdminGroup}); err != nil {
			err = errors.Annotate(err, "aborting groups import").Err()
			logging.Errorf(ctx, err.Error())
			return nil, revision, err
		}

		// Land the change iff the current AuthDB revision is still == `revision`.
		err := applyImport(revision, entitiesToPut, entitiesToDel, ts)
		if err != nil {
			if errors.Is(err, ErrConcurrentAuthDBUpdate) {
				// Retry because the AuthDB changed between transactions.
				logging.Warningf(ctx, "%s - retrying...", err.Error())
				continue
			}

			logging.Errorf(ctx, "couldn't apply changes to datastore entities %s", err.Error())
			return nil, revision, err
		}

		// The new revision has landed.
		revision += 1

		// Check if there are still changes to land.
		if truncated {
			logging.Infof(ctx, "going for another round to push the rest of the groups...")
			clock.Sleep(ctx, 5*time.Second)
			continue
		}

		logging.Infof(ctx, "Done")
		break
	}

	if len(updatedGroups) > 0 {
		return updatedGroups.ToSortedSlice(), revision, nil
	}

	return nil, 0, nil
}

// prepareImport compares the bundle given to the what is currently present in datastore
// to get the operations for all the groups.
func prepareImport(ctx context.Context, systemName string, existingGroups map[string]*AuthGroup, iGroups GroupBundle,
	providedBy identity.Identity, createdTS time.Time) (toPut []*AuthGroup, toDel []*AuthGroup) {
	// Filter existing groups to those that belong to the given system.
	sysGroupsSet := stringset.New(0)
	sysPrefix := fmt.Sprintf("%s/", systemName)
	for gID := range existingGroups {
		if strings.HasPrefix(gID, sysPrefix) {
			sysGroupsSet.Add(gID)
		}
	}

	// Get the unique group names in this system's bundle.
	iGroupsSet := stringset.New(len(iGroups))
	for groupName := range iGroups {
		iGroupsSet.Add(groupName)
	}

	// Create new groups.
	toCreate := iGroupsSet.Difference(sysGroupsSet).ToSlice()
	for _, g := range toCreate {
		group := makeAuthGroup(ctx, g)
		group.Members = identitiesToStrings(iGroups[g])
		group.Owners = AdminGroup
		group.CreatedBy = string(providedBy)
		group.CreatedTS = createdTS
		toPut = append(toPut, group)
	}

	// Update existing groups.
	toUpdate := sysGroupsSet.Intersect(iGroupsSet).ToSlice()
	for _, g := range toUpdate {
		existingGroup := existingGroups[g]
		importGMems := stringset.NewFromSlice(identitiesToStrings(iGroups[g])...)
		existMems := existingGroup.Members
		if len(importGMems) != len(existMems) || !importGMems.HasAll(existMems...) {
			existingGroup.Members = importGMems.ToSortedSlice()
			toPut = append(toPut, existingGroup)
		}
	}

	// Delete or clear members of groups that are no longer present in the
	// bundle. If the group is referenced somewhere, just clear its members list
	// to avoid inconsistency in group inclusion graphs.
	toDelete := sysGroupsSet.Difference(iGroupsSet).ToSlice()
	for _, groupName := range toDelete {
		isSubgroup := false
		for _, existingGroup := range existingGroups {
			if contains(groupName, existingGroup.Nested) {
				isSubgroup = true
				break
			}
		}

		existingGroup := existingGroups[groupName]
		if isSubgroup {
			// The group is a subgroup of another so it shouldn't be deleted.
			// Clear members only if there are currently members in the group.
			if len(existingGroup.Members) != 0 {
				existingGroup.Members = []string{}
				toPut = append(toPut, existingGroup)
			}
			continue
		}

		toDel = append(toDel, existingGroup)
	}

	return toPut, toDel
}

func identitiesToStrings(idents []identity.Identity) []string {
	res := make([]string, len(idents))
	for i, id := range idents {
		res[i] = string(id)
	}
	return res
}

// extractTarArchive unpacks a tar archive and returns a map
// of filename -> fileobj pairs.
func extractTarArchive(r io.Reader) (map[string][]byte, error) {
	entries := make(map[string][]byte)
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		fileContents, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		entries[header.Name] = fileContents
	}

	if err := gzr.Close(); err != nil {
		return nil, err
	}
	return entries, nil
}

// TODO(cjacomet): replace with slices.Contains when
// slices package isn't experimental.
func contains(key string, search []string) bool {
	for _, val := range search {
		if val == key {
			return true
		}
	}
	return false
}

// ToProto converts the GroupImporterConfig entity to the proto equivalent.
func (g *GroupImporterConfig) ToProto() (*configspb.GroupImporterConfig, error) {
	gConfig := &configspb.GroupImporterConfig{}
	if err := prototext.Unmarshal([]byte(g.ConfigProto), gConfig); err != nil {
		return nil, err
	}
	return gConfig, nil
}
