// Copyright 2016 The LUCI Authors.
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

package buildbot

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"

	"golang.org/x/net/context"
)

func decodeMasterEntry(
	c context.Context, entry *buildbotMasterEntry, master *buildbotMaster) error {

	reader, err := gzip.NewReader(bytes.NewReader(entry.Data))
	if err != nil {
		return err
	}
	defer reader.Close()
	if err = json.NewDecoder(reader).Decode(master); err != nil {
		return err
	}
	return nil
}

// canAccessMaster returns nil iff the currently logged in user is able to see
// internal masters, or if the given master is a known public master.
func canAccessMaster(c context.Context, name string) error {
	cu := auth.CurrentUser(c)
	anon := cu.Identity == identity.AnonymousIdentity
	if !anon {
		// If we're logged in, and we can see internal stuff, return nil.
		//
		// getMasterEntry will maybe return 404 later if the master doesn't actually
		// exist.
		if allowed, err := common.IsAllowedInternal(c); err != nil || allowed {
			return err
		}
	}

	// We're not logged in, or we can only see public stuff, so see if the master
	// is public.
	if err := ds.Get(c, &buildbotMasterPublic{name}); err == nil {
		// It exists and is public
		return nil
	}

	if anon {
		// They need to log in before we can tell them more stuff.
		return errNotAuth
	}

	// They are logged in but have no access, so tell them it's missing.
	return errMasterNotFound
}

// getMasterEntry feches the named master and does an ACL check on the
// current user.
// It returns:
func getMasterEntry(c context.Context, name string) (*buildbotMasterEntry, error) {
	if err := canAccessMaster(c, name); err != nil {
		return nil, err
	}

	entry := buildbotMasterEntry{Name: name}
	err := ds.Get(c, &entry)
	if err == ds.ErrNoSuchEntity {
		err = errMasterNotFound
	}
	return &entry, err
}

// getMasterJSON fetches the latest known buildbot master data and returns
// the buildbotMaster struct (if found), whether or not it is internal,
// the last modified time, and an error if not found.
func getMasterJSON(c context.Context, name string) (
	master *buildbotMaster, internal bool, t time.Time, err error) {
	master = &buildbotMaster{}
	entry, err := getMasterEntry(c, name)
	if err != nil {
		return
	}
	t = entry.Modified
	internal = entry.Internal
	err = decodeMasterEntry(c, entry, master)
	return
}

// GetAllBuilders returns a resp.Module object containing all known masters
// and builders.
func GetAllBuilders(c context.Context) (*resp.CIService, error) {
	result := &resp.CIService{Name: "Buildbot"}
	// Fetch all Master entries from datastore
	q := ds.NewQuery("buildbotMasterEntry")
	// TODO(hinoka): Maybe don't look past like a month or so?
	entries := []*buildbotMasterEntry{}
	err := (&ds.Batcher{}).GetAll(c, q, &entries)
	if err != nil {
		return nil, err
	}

	// Add each builder from each master entry into the result.
	// TODO(hinoka): FanInOut this?
	for _, entry := range entries {
		if entry.Internal {
			// Bypass the master if it's an internal master and the user is not
			// part of the buildbot-private project.
			allowed, err := common.IsAllowedInternal(c)
			if err != nil {
				logging.WithError(err).Errorf(c, "Could not process master %s", entry.Name)
				return nil, err
			}
			if !allowed {
				continue
			}
		}
		master := &buildbotMaster{}
		err = decodeMasterEntry(c, entry, master)
		if err != nil {
			logging.WithError(err).Errorf(c, "Could not decode %s", entry.Name)
			continue
		}
		ml := resp.BuilderGroup{Name: entry.Name}
		// Sort the builder listing.
		sb := make([]string, 0, len(master.Builders))
		for bn := range master.Builders {
			sb = append(sb, bn)
		}
		sort.Strings(sb)
		for _, bn := range sb {
			// Go templates escapes this for us, and also
			// slashes are not allowed in builder names.
			ml.Builders = append(ml.Builders, *resp.NewLink(
				bn, fmt.Sprintf("/buildbot/%s/%s", entry.Name, bn)))
		}
		result.BuilderGroups = append(result.BuilderGroups, ml)
	}
	return result, nil
}
