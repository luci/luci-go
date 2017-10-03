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

type Master struct {
	buildbot.Master
	Internal bool
	Modified time.Time
}

// GetMaster fetches the latest known buildbot masterEntity state and
// the last modified time.
// If the masterEntity is not accessible to the current user, return an error.
//
// If refreshState is true, refreshes individual builds from the datastore
// and the list of Cached builds.
func GetMaster(c context.Context, name string, refreshState bool) (*Master, error) {
	entity := masterEntity{Name: name}
	err := datastore.Get(c, &entity)
	if err == datastore.ErrNoSuchEntity {
		return nil, errors.New("masterEntity not found", common.CodeNotFound)
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

	var refreshBuilds []*buildEntity
	for _, slave := range m.Slaves {
		for _, b := range slave.Runningbuilds {
			refreshBuilds = append(refreshBuilds, (*buildEntity)(b))
		}
	}
	if err = datastore.Get(c, refreshBuilds); err != nil {
		return nil, errors.Annotate(err, "refresh builds").Err()
	}

	// Also inject cached builds information.
	return m, parallel.FanOutIn(func(work chan<- func() error) {
		for builderName, builder := range m.Builders {
			builderName := builderName
			builder := builder
			work <- func() error {
				// Get the most recent 50 buildNums on the builder to simulate what the
				// cachedBuilds field looks like from the real buildbot masterEntity json.
				q := datastore.NewQuery("buildbotBuild").
					Eq("finished", true).
					Eq("masterEntity", name).
					Eq("builder", builderName).
					Limit(50).
					Order("-number").
					KeysOnly(true)
				var builds []*buildEntity
				if err := datastore.GetAll(c, q, &builds); err != nil {
					return err
				}
				builder.CachedBuilds = make([]int, len(builds))
				for i, b := range builds {
					builder.CachedBuilds[i] = b.Number
				}
				return nil
			}
		}
	})
}

// AllMasters returns all buildbot masters.
func AllMasters(c context.Context, checkAccess bool) ([]*Master, error) {
	// masterQueryBatchSize is the batch size to use when querying masters.
	const masterQueryBatchSize = int32(500)
	allowInternal := isAllowedInternal(c)
	masters := make([]*Master, 0, masterQueryBatchSize)
	q := datastore.NewQuery(masterKind)
	err := datastore.RunBatch(c, masterQueryBatchSize, q, func(e *masterEntity) error {
		if !checkAccess || allowInternal || !e.Internal {
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

func ImportMaster(
	c context.Context, master *buildbot.Master, internal bool) error {
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
	entry := masterEntity{
		Name:     master.Name,
		Internal: internal,
		Modified: clock.Now(c).UTC(),
	}
	toPut := []interface{}{&entry}
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
	entry.Data, err = encode(master)
	if err != nil {
		return err
	}
	logging.Debugf(c, "Length of gzipped data: %d", len(entry.Data))
	// Limit for datastore_v3 is 1572864 bytes for the total datastore entry.
	if len(entry.Data) > 1024*1024 {
		// FIXME: there should have been a metric and alert.
		logging.Errorf(c, "Size of master data too large, dropping")
		return nil
	}
	return datastore.Put(c, toPut)
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
		return false
	}
	return allowed
}

// CanAccessMaster returns nil iff the currently logged in user is able to see
// internal masters, or if the given masterEntity is a known public masterEntity.
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
	// Act like masterEntity does not exist.
	return errors.Reason("master %q not found", name).Tag(code).Err()
}

var masterKind = "buildbotMasterEntry"

// masterEntity is a datastore entity containing marshaled and packed buildbot
// masterEntity json.
type masterEntity struct {
	_kind string `gae:"$kind,buildbotMasterEntry"`
	// Name of the buildbot masterEntity.
	Name string `gae:"$id"`
	// Internal indicates whether the masterEntity is internal.
	// This value must by synced with the existance of masterPublic entity.
	// FIXME: the masterPublic entity should have been in the same entity group.
	Internal bool
	// Data is the json serialized and gzipped blob of the masterEntity data.
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
