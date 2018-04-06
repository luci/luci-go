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

// OS represents a row in the oses table.
type OS struct {
	config.OS
	Id int64
}

// OSesTable represents the table of operating systems in the database.
type OSesTable struct {
	// current is the slice of operating systems in the database.
	current []*OS

	// additions is a slice of operating systems pending addition to the database.
	additions []*OS
	// removals is a slice of operating systems pending removal from the database.
	removals []*OS
	// updates is a slice of operating systems pending update in the database.
	updates []*OS
}

// fetch fetches the operating systems from the database.
func (t *OSesTable) fetch(c context.Context) error {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT id, name, description
		FROM oses
	`)
	if err != nil {
		return errors.Annotate(err, "failed to select operating systems").Err()
	}
	defer rows.Close()
	for rows.Next() {
		os := &OS{}
		if err := rows.Scan(&os.Id, &os.Name, &os.Description); err != nil {
			return errors.Annotate(err, "failed to scan operating system").Err()
		}
		t.current = append(t.current, os)
	}
	return nil
}

// needsUpdate returns true if the given row needs to be updated to match the given config.
func (*OSesTable) needsUpdate(row, cfg *OS) bool {
	return row.Description != cfg.Description
}

// computeChanges computes the changes that need to be made to the operating systems in the database.
func (t *OSesTable) computeChanges(c context.Context, oses []*config.OS) {
	cfgs := make(map[string]*OS, len(oses))
	for _, cfg := range oses {
		cfgs[cfg.Name] = &OS{
			OS: config.OS{
				Name:        cfg.Name,
				Description: cfg.Description,
			},
		}
	}

	for _, os := range t.current {
		if cfg, ok := cfgs[os.Name]; ok {
			// Operating system found in the config.
			if t.needsUpdate(os, cfg) {
				// Operating system doesn't match the config.
				cfg.Id = os.Id
				t.updates = append(t.updates, cfg)
			}
			// Record that the operating system config has been seen.
			delete(cfgs, cfg.Name)
		} else {
			// Operating system not found in the config.
			t.removals = append(t.removals, os)
		}
	}

	// Operating systems remaining in the map are present in the config but not the database.
	// Iterate deterministically over the slice to determine which operating systems need to be added.
	for _, cfg := range oses {
		if os, ok := cfgs[cfg.Name]; ok {
			t.additions = append(t.additions, os)
		}
	}
}

// add adds all operating systems pending addition to the database, clearing pending additions.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *OSesTable) add(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.additions) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		INSERT INTO oses (name, description)
		VALUES (?, ?)
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Add each operating system to the database, and update the slice of operating systems with each addition.
	for len(t.additions) > 0 {
		os := t.additions[0]
		result, err := stmt.ExecContext(c, os.Name, os.Description)
		if err != nil {
			return errors.Annotate(err, "failed to add operating system %q", os.Name).Err()
		}
		t.current = append(t.current, os)
		t.additions = t.additions[1:]
		logging.Infof(c, "Added operating system %q", os.Name)
		os.Id, err = result.LastInsertId()
		if err != nil {
			return errors.Annotate(err, "failed to get operating system ID %q", os.Name).Err()
		}
	}
	return nil
}

// remove removes all operating systems pending removal from the database, clearing pending removals.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *OSesTable) remove(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.removals) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		DELETE FROM oses
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Remove each operating system from the table. It's more efficient to update the slice of
	// operating systems once at the end rather than for each removal, so use a defer.
	removed := make(map[int64]struct{}, len(t.removals))
	defer func() {
		var oses []*OS
		for _, os := range t.current {
			if _, ok := removed[os.Id]; !ok {
				oses = append(oses, os)
			}
		}
		t.current = oses
	}()
	for len(t.removals) > 0 {
		os := t.removals[0]
		if _, err := stmt.ExecContext(c, os.Id); err != nil {
			// Defer ensures the slice of operating systems is updated even if we exit early.
			return errors.Annotate(err, "failed to remove operating system %q", os.Name).Err()
		}
		removed[os.Id] = struct{}{}
		t.removals = t.removals[1:]
		logging.Infof(c, "Removed operating system %q", os.Name)
	}
	return nil
}

// update updates all operating systems pending update in the database, clearing pending updates.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *OSesTable) update(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.updates) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		UPDATE oses
		SET description = ?
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Update each operating system in the table. It's more efficient to update the slice of
	// operating systems once at the end rather than for each update, so use a defer.
	updated := make(map[int64]*OS, len(t.updates))
	defer func() {
		for _, os := range t.current {
			if u, ok := updated[os.Id]; ok {
				os.Description = u.Description
			}
		}
	}()
	for len(t.updates) > 0 {
		os := t.updates[0]
		if _, err := stmt.ExecContext(c, os.Description, os.Id); err != nil {
			return errors.Annotate(err, "failed to update operating system %q", os.Name).Err()
		}
		updated[os.Id] = os
		t.updates = t.updates[1:]
		logging.Infof(c, "Updated operating system %q", os.Name)
	}
	return nil
}

// ids returns a map of operating system names to IDs.
func (t *OSesTable) ids(c context.Context) map[string]int64 {
	oses := make(map[string]int64, len(t.current))
	for _, os := range t.current {
		oses[os.Name] = os.Id
	}
	return oses
}

// EnsureOSes ensures the database contains exactly the given operating systems.
func EnsureOSes(c context.Context, cfgs []*config.OS) error {
	t := &OSesTable{}
	if err := t.fetch(c); err != nil {
		return errors.Annotate(err, "failed to fetch operating systems").Err()
	}
	t.computeChanges(c, cfgs)
	if err := t.add(c); err != nil {
		return errors.Annotate(err, "failed to add operating systems").Err()
	}
	if err := t.remove(c); err != nil {
		return errors.Annotate(err, "failed to remove operating systems").Err()
	}
	if err := t.update(c); err != nil {
		return errors.Annotate(err, "failed to update operating systems").Err()
	}
	return nil
}
