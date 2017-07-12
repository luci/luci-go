// Copyright 2015 The LUCI Authors.
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

// Package gaesettings implements settings.Storage interface on top of GAE
// datastore.
//
// See github.com/luci/luci-go/server/settings for more details.
package gaesettings

import (
	"encoding/json"
	"strconv"
	"time"

	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/server/settings"
)

// Storage knows how to store JSON blobs with settings in the datastore.
//
// It implements server/settings.EventualConsistentStorage interface.
type Storage struct{}

// settingsEntity is used to store all settings as JSON blob. Latest settings
// are stored under key (gaesettings.Settings, latest). The historical log is
// stored using exact same entity under keys (gaesettings.SettingsLog, version),
// with parent being (gaesettings.Settings, latest). Version is monotonically
// increasing integer starting from 1.
type settingsEntity struct {
	Kind    string    `gae:"$kind"`
	ID      string    `gae:"$id"`
	Parent  *ds.Key   `gae:"$parent"`
	Version int       `gae:",noindex"`
	Value   string    `gae:",noindex"`
	Who     string    `gae:",noindex"`
	Why     string    `gae:",noindex"`
	When    time.Time `gae:",noindex"`

	// Disable dscache, since settings must remain functional in case memcache is
	// malfunctioning.
	_ ds.Toggle `gae:"$dscache.enable,false"`
}

// defaultContext returns datastore interface configured to use default
// namespace and escape any current transaction.
func defaultContext(c context.Context) context.Context {
	return ds.WithoutTransaction(info.MustNamespace(c, ""))
}

// latestSettings returns settingsEntity with prefilled key pointing to latest
// settings.
func latestSettings() settingsEntity {
	return settingsEntity{Kind: "gaesettings.Settings", ID: "latest"}
}

// expirationDuration returns how long to hold settings in memory cache.
//
// One minute in prod, one second on dev server (since long expiration time on
// dev server is very annoying).
func (s Storage) expirationDuration(c context.Context) time.Duration {
	if info.IsDevAppServer(c) {
		return time.Second
	}
	return time.Minute
}

// FetchAllSettings fetches all latest settings at once.
func (s Storage) FetchAllSettings(c context.Context) (*settings.Bundle, error) {
	c = defaultContext(c)
	logging.Debugf(c, "Fetching app settings from the datastore")

	latest := latestSettings()
	switch err := ds.Get(c, &latest); {
	case err == ds.ErrNoSuchEntity:
		break
	case err != nil:
		return nil, transient.Tag.Apply(err)
	}

	pairs := map[string]*json.RawMessage{}
	if latest.Value != "" {
		if err := json.Unmarshal([]byte(latest.Value), &pairs); err != nil {
			return nil, err
		}
	}
	return &settings.Bundle{
		Values: pairs,
		Exp:    clock.Now(c).Add(s.expirationDuration(c)),
	}, nil
}

// UpdateSetting updates a setting at the given key.
func (s Storage) UpdateSetting(c context.Context, key string, value json.RawMessage, who, why string) error {
	c = defaultContext(c)

	var fatalFail error // set in transaction on fatal errors
	err := ds.RunInTransaction(c, func(c context.Context) error {
		// Fetch the most recent values.
		latest := latestSettings()
		if err := ds.Get(c, &latest); err != nil && err != ds.ErrNoSuchEntity {
			return err
		}

		// Update the value.
		pairs := map[string]*json.RawMessage{}
		if len(latest.Value) != 0 {
			if err := json.Unmarshal([]byte(latest.Value), &pairs); err != nil {
				fatalFail = err
				return err
			}
		}
		pairs[key] = &value

		// Store the previous one in the log.
		auditCopy := latest
		auditCopy.Kind = "gaesettings.SettingsLog"
		auditCopy.ID = strconv.Itoa(latest.Version)
		auditCopy.Parent = ds.KeyForObj(c, &latest)

		// Prepare a new version.
		buf, err := json.MarshalIndent(pairs, "", "  ")
		if err != nil {
			fatalFail = err
			return err
		}
		latest.Version++
		latest.Value = string(buf)
		latest.Who = who
		latest.Why = why
		latest.When = clock.Now(c).UTC()

		// Skip update if no changes at all.
		if latest.Value == auditCopy.Value {
			return nil
		}

		// Don't store copy of "no settings at all", it's useless.
		if latest.Version == 1 {
			return ds.Put(c, &latest)
		}
		return ds.Put(c, &latest, &auditCopy)
	}, nil)

	if fatalFail != nil {
		return fatalFail
	}
	return transient.Tag.Apply(err)
}

// GetConsistencyTime returns "last modification time" + "expiration period".
//
// It indicates moment in time when last setting change is fully propagated to
// all instances.
//
// Returns zero time if there are no settings stored.
func (s Storage) GetConsistencyTime(c context.Context) (time.Time, error) {
	c = defaultContext(c)
	latest := latestSettings()
	switch err := ds.Get(c, &latest); err {
	case nil:
		return latest.When.Add(s.expirationDuration(c)), nil
	case ds.ErrNoSuchEntity:
		return time.Time{}, nil
	default:
		return time.Time{}, transient.Tag.Apply(err)
	}
}
