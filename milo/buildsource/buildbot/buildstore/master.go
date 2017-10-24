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
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/auth"
)

// Master is buildbot.Master plus extra storage-level information.
type Master struct {
	buildbot.Master
	Internal bool
	Modified time.Time
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
		return nil, errors.New("master not found", common.CodeNotFound)
	}
	if err != nil {
		return nil, err
	}

	m, err := entity.decode()
	if err != nil {
		return nil, err
	}
	if !refreshState {
		return m, nil
	}

	emulation := false
	for builder := range m.Builders {
		opt, err := GetEmulationOptions(c, name, builder)
		if err != nil {
			return nil, err
		}
		if opt != nil {
			emulation = true
			break
		}
	}
	if emulation {
		// Emulation does not support this Slaves field.
		m.Slaves = nil
		for _, b := range m.Builders {
			b.Slaves = nil
			b.PendingBuildStates = nil
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
	return m, parallel.FanOutIn(func(work chan<- func() error) {
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

				opt, err := GetEmulationOptions(c, name, builderName)
				if err != nil {
					return err
				}
				if opt != nil {
					// Pending buildbot builds do not have a number yet
					// so it is hard to tell whether they WILL be before
					// or after opt.StartFrom. Just ignore them.
					builder.PendingBuilds = 0
					// But keep current ones because they have numbers
					// and it is easier to decide whether should be included in the
					// response or not.
					buildbotCurrent := builder.CurrentBuilds
					builder.CurrentBuilds = nil

					// Load all incomplete builds and add only those after
					// opt.StartFrom.
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
					for _, b := range res.Builds {
						if b.Number < int(opt.StartFrom) {
							// the rest of res.Builds are buildbot builds
							// ignore them.
							break
						}
						if b.Times.Start.IsZero() {
							builder.PendingBuilds++
						} else {
							builder.CurrentBuilds = append(builder.CurrentBuilds, b.Number)
						}
					}
					for _, num := range buildbotCurrent {
						if num < int(opt.StartFrom) {
							builder.CurrentBuilds = append(builder.CurrentBuilds, num)
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
			m, err := e.decode()
			if err != nil {
				return err
			}
			masters = append(masters, m)
		}
		return nil
	})
	return masters, err
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

	for _, builder := range master.Builders {
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
	}
	entity := masterEntity{
		Name:     master.Name,
		Internal: internal,
		Modified: clock.Now(c).UTC(),
	}
	toPut := []interface{}{&entity}
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
			Tag(ErrTooBig).
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
	return parallel.WorkPool(10, func(work chan<- func() error) {
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

	code := common.CodeNotFound
	if auth.CurrentUser(c).Identity == identity.AnonymousIdentity {
		code = common.CodeUnauthorized
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
	// This value must by synced with the existance of masterPublic entity.
	// FIXME: the masterPublic entity should have been in the same entity group.
	Internal bool
	// Data is the json serialized and gzipped buildbot.Master.
	Data []byte `gae:",noindex"`
	// Modified is when this entry was last modified.
	Modified time.Time
}

func (m *masterEntity) decode() (*Master, error) {
	res := Master{
		Internal: m.Internal,
		Modified: m.Modified,
	}
	return &res, decode(&res.Master, m.Data)
}
