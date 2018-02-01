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

package model

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// Platform represents a platform.
type Platform struct {
	config.Platform
	Id int64
}

// PlatformsTable represents the table of platforms in the database.
type PlatformsTable struct {
	// current is the slice of platforms in the database.
	current []*Platform

	// additions is a slice of platforms pending addition to the database.
	additions []*Platform
	// removals is a slice of platforms pending removal from the database.
	removals []*Platform
	// updates is a slice of platforms pending update in the database.
	updates []*Platform
}

// fetch fetches the platforms from the database.
func (t *PlatformsTable) fetch(c context.Context) error {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT id, name, description, state
		FROM platforms
	`)
	if err != nil {
		return errors.Annotate(err, "failed to select platforms").Err()
	}
	defer rows.Close()
	for rows.Next() {
		p := &Platform{}
		if err := rows.Scan(&p.Id, &p.Name, &p.Description, &p.State); err != nil {
			return errors.Annotate(err, "failed to scan platform").Err()
		}
		t.current = append(t.current, p)
	}
	return nil
}

// needsUpdate returns true if the given row needs to be updated to match the given config.
func (*PlatformsTable) needsUpdate(row, cfg *Platform) bool {
	return row.Description != cfg.Description || row.State != cfg.State
}

// computeChanges computes the changes that need to be made to the platforms in the database.
func (t *PlatformsTable) computeChanges(c context.Context, platforms []*config.Platform) {
	cfgs := make(map[string]*Platform, len(platforms))
	for _, cfg := range platforms {
		cfgs[cfg.Name] = &Platform{
			Platform: config.Platform{
				Name:        cfg.Name,
				Description: cfg.Description,
				State:       cfg.State,
			},
		}
	}

	for _, p := range t.current {
		if cfg, ok := cfgs[p.Name]; ok {
			// Platform found in the config.
			if t.needsUpdate(p, cfg) {
				// Platform doesn't match the config.
				cfg.Id = p.Id
				t.updates = append(t.updates, cfg)
			}
			// Record that the platform config has been seen.
			delete(cfgs, cfg.Name)
		} else {
			// Platform not found in the config.
			t.removals = append(t.removals, p)
		}
	}

	// Platforms remaining in the map are present in the config but not the database.
	// Iterate deterministically over the slice to determine which platforms need to be added.
	for _, cfg := range platforms {
		if p, ok := cfgs[cfg.Name]; ok {
			t.additions = append(t.additions, p)
		}
	}
}

// add adds all platforms pending addition to the database, clearing pending additions.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *PlatformsTable) add(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.additions) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		INSERT INTO platforms (name, description, state)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Add each platform to the database, and update the slice of platforms with each addition.
	for len(t.additions) > 0 {
		p := t.additions[0]
		result, err := stmt.ExecContext(c, p.Name, p.Description, p.State)
		if err != nil {
			return errors.Annotate(err, "failed to add platform %q", p.Name).Err()
		}
		t.current = append(t.current, p)
		t.additions = t.additions[1:]
		logging.Infof(c, "Added platform %q", p.Name)
		p.Id, err = result.LastInsertId()
		if err != nil {
			return errors.Annotate(err, "failed to get platform ID %q", p.Name).Err()
		}
	}
	return nil
}

// remove removes all platforms pending removal from the database, clearing pending removals.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *PlatformsTable) remove(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.removals) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		DELETE FROM platforms
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Remove each platforms from the database. It's more efficient to update the slice of
	// platforms once at the end rather than for each removal, so use a defer.
	removed := make(map[int64]struct{}, len(t.removals))
	defer func() {
		var platforms []*Platform
		for _, p := range t.current {
			if _, ok := removed[p.Id]; !ok {
				platforms = append(platforms, p)
			}
		}
		t.current = platforms
	}()
	for len(t.removals) > 0 {
		p := t.removals[0]
		if _, err := stmt.ExecContext(c, p.Id); err != nil {
			// Defer ensures the slice of platforms is updated even if we exit early.
			return errors.Annotate(err, "failed to remove platform %q", p.Name).Err()
		}
		removed[p.Id] = struct{}{}
		t.removals = t.removals[1:]
		logging.Infof(c, "Removed platform %q", p.Name)
	}
	return nil
}

// update updates all platforms pending update in the database, clearing pending updates.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *PlatformsTable) update(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.updates) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		UPDATE platforms
		SET description = ?, state = ?
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Update each platform in the database. It's more efficient to update the slice of
	// platforms once at the end rather than for each update, so use a defer.
	updated := make(map[int64]*Platform, len(t.updates))
	defer func() {
		for _, p := range t.current {
			if u, ok := updated[p.Id]; ok {
				p.Description = u.Description
				p.State = u.State
			}
		}
	}()
	for len(t.updates) > 0 {
		p := t.updates[0]
		if _, err := stmt.ExecContext(c, p.Description, p.State, p.Id); err != nil {
			return errors.Annotate(err, "failed to update platform %q", p.Name).Err()
		}
		updated[p.Id] = p
		t.updates = t.updates[1:]
		logging.Infof(c, "Updated platform %q", p.Name)
	}
	return nil
}

// ids returns a map of platform names to IDs.
func (t *PlatformsTable) ids(c context.Context) map[string]int64 {
	platforms := make(map[string]int64, len(t.current))
	for _, p := range t.current {
		platforms[p.Name] = p.Id
	}
	return platforms
}

// EnsurePlatforms ensures the database contains exactly the given platforms.
func EnsurePlatforms(c context.Context, cfgs []*config.Platform) error {
	t := &PlatformsTable{}
	if err := t.fetch(c); err != nil {
		return errors.Annotate(err, "failed to fetch platforms").Err()
	}
	t.computeChanges(c, cfgs)
	if err := t.add(c); err != nil {
		return errors.Annotate(err, "failed to add platforms").Err()
	}
	if err := t.remove(c); err != nil {
		return errors.Annotate(err, "failed to remove platforms").Err()
	}
	if err := t.update(c); err != nil {
		return errors.Annotate(err, "failed to update platforms").Err()
	}
	return nil
}
