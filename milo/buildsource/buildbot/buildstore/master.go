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

package buildstore

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/auth"
)

// MasterExpiry is how long a master entity can be stale before we consider it to be expired.
const MasterExpiry = 4 * time.Hour

// Master is buildbot.Master plus extra storage-level information.
type Master struct {
	buildbot.Master
	Internal bool
	Modified time.Time
}

// getLUCIBuilders returns all LUCI builders for a given master from Swarmbucket.
// LUCI builders do not have their own concept of "master".  Instead, this is
// inferred from the "mastername" property in the property JSON.
func getLUCIBuilders(c context.Context, master string) ([]string, error) {
	buildersResponse, err := buildbucket.GetBuilders(c)
	if err != nil {
		return nil, err
	}

	var result []string
	for _, bucket := range buildersResponse.Buckets {
		for _, builder := range bucket.Builders {
			if builder.PropertiesJson == "" {
				continue
			}
			var prop struct {
				Mastername string `json:"mastername"`
			}
			if err := json.Unmarshal([]byte(builder.PropertiesJson), &prop); err != nil {
				logging.WithError(err).Errorf(c, "processing %s/%s", bucket.Name, builder.Name)
				continue
			}
			if prop.Mastername == master {
				result = append(result, builder.Name)
			}
		}
	}
	return result, nil
}

// GetMaster fetches a master.
//
// If refreshState is true, refreshes individual builds from the datastore
// and the list of Cached builds.
//
// If any of the master's builders is emulated, the returned Master
// does not have any slave information or pending build states.
//
// Does not check access.
func GetMaster(c context.Context, name string, refreshState bool) (*Master, error) {
	entity := masterEntity{Name: name}
	err := datastore.Get(c, &entity)
	if err == datastore.ErrNoSuchEntity {
		return nil, errors.New("master not found", grpcutil.NotFoundTag)
	}
	if err != nil {
		return nil, err
	}

	m, err := entity.decode(c)
	if err != nil {
		return nil, err
	}
	if !refreshState {
		return m, nil
	}

	emulation := EmulationEnabled(c)
	if emulation {
		// Emulation does not support this Slaves field.
		m.Slaves = nil
		for _, b := range m.Builders {
			b.Slaves = nil
			b.PendingBuildStates = nil
		}
		// Add in pure-luci builders, if not found in buildbot.
		builders, err := getLUCIBuilders(c, name)
		if err != nil {
			return nil, err
		}
		for _, builder := range builders {
			if _, ok := m.Builders[builder]; !ok {
				m.Builders[builder] = &buildbot.Builder{Buildername: builder}
			}
		}

	} else {
		var refreshBuilds []*buildEntity
		for _, slave := range m.Slaves {
			for _, b := range slave.Runningbuilds {
				refreshBuilds = append(refreshBuilds, (*buildEntity)(b))
			}
		}
		if err = datastore.Get(c, refreshBuilds); err != nil {
			return nil, errors.Annotate(err, "refresh builds").Err()
		}
	}

	// Inject cached builds information.
	return m, parallel.WorkPool(4, func(work chan<- func() error) {
		for builderName, builder := range m.Builders {
			builderName := builderName
			builder := builder
			work <- func() error {
				// Get the most recent 50 buildNums on the builder to simulate what the
				// cachedBuilds field looks like from the real buildbot master json.
				q := Query{
					Master:   name,
					Builder:  builderName,
					Finished: Yes,
					Limit:    50,

					NoAnnotationFetch: true,
					KeyOnly:           true,
				}
				res, err := GetBuilds(c, q)
				if err != nil {
					return err
				}
				builder.CachedBuilds = make([]int, len(res.Builds))
				for i, b := range res.Builds {
					builder.CachedBuilds[i] = b.Number
				}

				if emulation {
					// builder.PendingBuilds and builder.CurrentBuilds
					// contain only buildbot data. Add LUCI data.

					// This will load both Buildbot and LUCI builds, merged.
					q := Query{
						Master:   name,
						Builder:  builderName,
						Finished: No,

						// we will ignore buildbot builds
						KeyOnly:           true,
						NoAnnotationFetch: true,
					}
					res, err := GetBuilds(c, q)
					if err != nil {
						return err
					}

					// Pending buildbot builds are "build requests".
					// They did not turn into builds yet and we don't know if
					// they will be come experimental or not. The moment one
					// turns into a build, it will excluded from
					// builder.PendingBuilds and will possibly not included
					// in CurrentBuilds in case it is experimental.
					// Thus, not zeroing builder.PendingBuilds.

					// In contrast to Golang, nil in JSON is not a valid array.
					// so do not set CurrentBuilds to nil.
					builder.CurrentBuilds = make([]int, 0, len(res.Builds))
					for _, b := range res.Builds {
						if b.Times.Start.IsZero() {
							builder.PendingBuilds++
						} else {
							builder.CurrentBuilds = append(builder.CurrentBuilds, b.Number)
						}
					}
				}

				return nil
			}
		}
	})
}

// AllMasters returns all buildbot masters.
func AllMasters(c context.Context, checkAccess bool) ([]*Master, error) {
	const batchSize = int32(500)
	masters := make([]*Master, 0, batchSize)
	q := datastore.NewQuery(masterKind)

	// note: avoid calling isAllowedInternal is checkAccess is false.
	// checkAccess is usually false in cron jobs where there is no auth state,
	// so isAllowedInternal call would unconditionally log an error.
	allowInternal := !checkAccess || isAllowedInternal(c)
	err := datastore.RunBatch(c, batchSize, q, func(e *masterEntity) error {
		if allowInternal || !e.Internal {
			m, err := e.decode(c)
			if err != nil {
				return err
			}
			masters = append(masters, m)
		}
		return nil
	})
	return masters, err
}

// GetPendingCounts returns numbers of pending builds in builders.
// builders must be a list of slash-separated master, builder names.
func GetPendingCounts(c context.Context, builders []string) ([]int, error) {
	entities := make([]builderEntity, len(builders))
	for i, b := range builders {
		parts := strings.SplitN(b, "/", 2)
		if len(parts) < 2 {
			return nil, errors.Reason("builder does not have a slash: %q", b).Err()
		}
		entities[i].MasterKey = datastore.MakeKey(c, masterKind, parts[0])
		entities[i].Name = parts[1]
	}
	if err := datastore.Get(c, entities); err != nil {
		for _, e := range err.(errors.MultiError) {
			if e != nil && e != datastore.ErrNoSuchEntity {
				return nil, err
			}
		}
	}

	counts := make([]int, len(entities))
	for i, e := range entities {
		counts[i] = e.PendingCount
	}
	return counts, nil
}

// PutPendingCount persists number of pending builds to a builder.
// Useful for testing.
func PutPendingCount(c context.Context, master, builder string, count int) error {
	return datastore.Put(c, &builderEntity{
		MasterKey:    datastore.MakeKey(c, masterKind, master),
		Name:         builder,
		PendingCount: count,
	})
}

// ExpireCallback is called when a build is marked as expired.
type ExpireCallback func(b *buildbot.Build, reason string)

// SaveMaster persists the master in the storage.
//
// Expires all incomplete builds in the datastore associated with this master
// and
//   - associated with builders not declared in master.Builders
//   - OR or not "current" from this master's perspective and >=20min stale.
func SaveMaster(c context.Context, master *buildbot.Master,
	internal bool, expireCallback ExpireCallback) error {

	entity := &masterEntity{
		Name:     master.Name,
		Internal: internal,
		Modified: clock.Now(c).UTC(),
	}
	toPut := []interface{}{entity}
	for builderName, builder := range master.Builders {
		// Trim out extra info in the "Changes" portion of the pending build state,
		// we don't actually need comments, files, and properties
		for _, pbs := range builder.PendingBuildStates {
			for i := range pbs.Source.Changes {
				c := &pbs.Source.Changes[i]
				c.Comments = ""
				c.Files = nil
				c.Properties = nil
			}
		}
		toPut = append(toPut, &builderEntity{
			MasterKey:    datastore.KeyForObj(c, entity),
			Name:         builderName,
			PendingCount: builder.PendingBuilds,
		})
	}
	publicTag := &masterPublic{Name: master.Name}
	if internal {
		// do the deletion immediately so that the 'public' bit is removed from
		// datastore before any internal details are actually written to datastore.
		if err := datastore.Delete(c, publicTag); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
	} else {
		toPut = append(toPut, publicTag)
	}

	var err error
	entity.Data, err = encode(master)
	if err != nil {
		return err
	}
	logging.Debugf(c, "Length of gzipped master data: %d", len(entity.Data))
	if len(entity.Data) > maxDataSize {
		return errors.Reason("master data is %d bytes, which is more than %d limit", len(entity.Data), maxDataSize).
			Tag(TooBigTag).
			Err()
	}
	if err := datastore.Put(c, toPut); err != nil {
		return err
	}

	return cleanUpExpiredBuilds(c, master, expireCallback)
}

func cleanUpExpiredBuilds(c context.Context, master *buildbot.Master, expiredCallback ExpireCallback) error {
	q := datastore.NewQuery(buildKind).
		Eq("master", master.Name).
		Eq("finished", false)
	return parallel.WorkPool(4, func(work chan<- func() error) {
		err := datastore.Run(c, q, func(b *buildEntity) {
			now := clock.Now(c)
			reason := ""
			if builder, ok := master.Builders[b.Buildername]; !ok {
				reason = "builder removed"
			} else {
				for _, bnum := range builder.CurrentBuilds {
					if b.Number == bnum {
						return
					}
				}
				// This build is not among master's current builds.
				// Expire builds after 20 minutes of not getting data.
				if now.Sub(b.TimeStamp.Time) > 20*time.Minute {
					reason = "build stale"
				}
			}
			if reason != "" {
				b := (*buildbot.Build)(b)
				work <- func() error {
					err := expireBuild(c, b)
					if err != nil {
						return err
					}
					if expiredCallback != nil {
						expiredCallback(b, reason)
					}
					return nil
				}
			}
		})
		if err != nil {
			// use the existing channel for returning errors
			work <- func() error { return err }
		}
	})
}

// expireBuild marks a build as finished and expired.
func expireBuild(c context.Context, b *buildbot.Build) error {
	if !b.TimeStamp.IsZero() {
		b.Times.Finish = b.TimeStamp
	} else {
		b.Times.Finish.Time = clock.Now(c)
	}
	b.Finished = true
	b.Results = buildbot.Exception
	b.Currentstep = nil
	b.Text = append(b.Text, "Build expired on Milo")
	_, err := SaveBuild(c, b)
	return err
}

// masterPublic is a datastore entity that exists for public builtbot masters,
// and not for internal masters. It's used for ACL checks.
type masterPublic struct {
	_kind string `gae:"$kind,buildbotMasterPublic"`
	Name  string `gae:"$id"`
}

// isAllowedInternal returns true if the current user has access to internal
// data. In case of an error, logs it and returns false to prevent from
// sniffing based on internal errors.
func isAllowedInternal(c context.Context) bool {
	settings := common.GetSettings(c)
	if settings.Buildbot.InternalReader == "" {
		return false
	}
	allowed, err := auth.IsMember(c, settings.Buildbot.InternalReader)
	if err != nil {
		logging.WithError(err).Errorf(c, "IsMember(%q) failed", settings.Buildbot.InternalReader)
		allowed = false
	}
	return allowed
}

// CanAccessMaster returns nil if the currently logged in user can see the
// masters, or if the given master is a known public master,
// otherwise an error.
func CanAccessMaster(c context.Context, name string) error {
	if ex, err := datastore.Exists(c, &masterPublic{Name: name}); err == nil && ex.Get(0) {
		// It exists => it is public
		return nil
	}

	if isAllowedInternal(c) {
		return nil
	}

	code := grpcutil.NotFoundTag
	if auth.CurrentUser(c).Identity == identity.AnonymousIdentity {
		code = grpcutil.UnauthenticatedTag
	}
	// Act like master does not exist.
	return errors.Reason("master %q not found", name).Tag(code).Err()
}

var masterKind = "buildbotMasterEntry"

// masterEntity is a datastore entity containing marshaled and compressed
// buildbot master json.
type masterEntity struct {
	_kind string `gae:"$kind,buildbotMasterEntry"`
	// Name of the buildbot master, without "master." prefix.
	Name string `gae:"$id"`
	// Internal indicates whether the master is internal.
	// This value must by synced with the existence of masterPublic entity.
	// FIXME: the masterPublic entity should have been in the same entity group.
	Internal bool
	// Data is the json serialized and gzipped buildbot.Master.
	Data []byte `gae:",noindex"`
	// Modified is when this entry was last modified.
	Modified time.Time
}

func (m *masterEntity) decode(c context.Context) (*Master, error) {
	deadline := m.Modified.Add(MasterExpiry)
	res := Master{
		Internal: m.Internal,
		Modified: m.Modified,
	}
	err := decode(&res.Master, m.Data)
	if err == nil && clock.Now(c).After(deadline) {
		// Purge the builder list if the master is expired.
		res.Master.Builders = map[string]*buildbot.Builder{}
	}
	return &res, err
}

// builderEntity is a child of masterEntity, specific to a Builder.
type builderEntity struct {
	_kind        string         `gae:"$kind,buildbotBuilder"`
	Name         string         `gae:"$id"` // builder name
	MasterKey    *datastore.Key `gae:"$parent"`
	PendingCount int
}
