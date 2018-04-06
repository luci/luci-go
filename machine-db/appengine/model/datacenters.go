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

// Datacenter represents a row in the datacenters table.
type Datacenter struct {
	config.Datacenter
	Id int64
}

// DatacentersTable represents the table of datacenters in the database.
type DatacentersTable struct {
	// current is the slice of datacenters in the database.
	current []*Datacenter

	// additions is a slice of datacenters pending addition to the database.
	additions []*Datacenter
	// removals is a slice of datacenters pending removal from the database.
	removals []*Datacenter
	// updates is a slice of datacenters pending update in the database.
	updates []*Datacenter
}

// fetch fetches the datacenters from the database.
func (t *DatacentersTable) fetch(c context.Context) error {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT id, name, description, state
		FROM datacenters
	`)
	if err != nil {
		return errors.Annotate(err, "failed to select datacenters").Err()
	}
	defer rows.Close()
	for rows.Next() {
		dc := &Datacenter{}
		if err := rows.Scan(&dc.Id, &dc.Name, &dc.Description, &dc.State); err != nil {
			return errors.Annotate(err, "failed to scan datacenter").Err()
		}
		t.current = append(t.current, dc)
	}
	return nil
}

// needsUpdate returns true if the given row needs to be updated to match the given config.
func (*DatacentersTable) needsUpdate(row, cfg *Datacenter) bool {
	return row.Description != cfg.Description || row.State != cfg.State
}

// computeChanges computes the changes that need to be made to the datacenters in the database.
func (t *DatacentersTable) computeChanges(c context.Context, datacenters []*config.Datacenter) {
	cfgs := make(map[string]*Datacenter, len(datacenters))
	for _, cfg := range datacenters {
		cfgs[cfg.Name] = &Datacenter{
			Datacenter: config.Datacenter{
				Name:        cfg.Name,
				Description: cfg.Description,
				State:       cfg.State,
			},
		}
	}

	for _, dc := range t.current {
		if cfg, ok := cfgs[dc.Name]; ok {
			// Datacenter found in the config.
			if t.needsUpdate(dc, cfg) {
				// Datacenter doesn't match the config.
				cfg.Id = dc.Id
				t.updates = append(t.updates, cfg)
			}
			// Record that the datacenter config has been seen.
			delete(cfgs, cfg.Name)
		} else {
			// Datacenter not found in the config.
			t.removals = append(t.removals, dc)
		}
	}

	// Datacenters remaining in the map are present in the config but not the database.
	// Iterate deterministically over the slice to determine which datacenters need to be added.
	for _, cfg := range datacenters {
		if dc, ok := cfgs[cfg.Name]; ok {
			t.additions = append(t.additions, dc)
		}
	}
}

// add adds all datacenters pending addition to the database, clearing pending additions.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *DatacentersTable) add(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.additions) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		INSERT INTO datacenters (name, description, state)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Add each datacenter to the database, and update the slice of datacenters with each addition.
	for len(t.additions) > 0 {
		dc := t.additions[0]
		result, err := stmt.ExecContext(c, dc.Name, dc.Description, dc.State)
		if err != nil {
			return errors.Annotate(err, "failed to add datacenter %q", dc.Name).Err()
		}
		t.current = append(t.current, dc)
		t.additions = t.additions[1:]
		logging.Infof(c, "Added datacenter %q", dc.Name)
		dc.Id, err = result.LastInsertId()
		if err != nil {
			return errors.Annotate(err, "failed to get datacenter ID %q", dc.Name).Err()
		}
	}
	return nil
}

// remove removes all datacenters pending removal from the database, clearing pending removals.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *DatacentersTable) remove(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.removals) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		DELETE FROM datacenters
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Remove each datacenter from the table. It's more efficient to update the slice of
	// datacenters once at the end rather than for each removal, so use a defer.
	removed := make(map[int64]struct{}, len(t.removals))
	defer func() {
		var dcs []*Datacenter
		for _, dc := range t.current {
			if _, ok := removed[dc.Id]; !ok {
				dcs = append(dcs, dc)
			}
		}
		t.current = dcs
	}()
	for len(t.removals) > 0 {
		dc := t.removals[0]
		if _, err := stmt.ExecContext(c, dc.Id); err != nil {
			// Defer ensures the slice of datacenters is updated even if we exit early.
			return errors.Annotate(err, "failed to remove datacenter %q", dc.Name).Err()
		}
		removed[dc.Id] = struct{}{}
		t.removals = t.removals[1:]
		logging.Infof(c, "Removed datacenter %q", dc.Name)
	}
	return nil
}

// update updates all datacenters pending update in the database, clearing pending updates.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *DatacentersTable) update(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.updates) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		UPDATE datacenters
		SET description = ?, state = ?
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Update each datacenter in the table. It's more efficient to update the slice of
	// datacenters once at the end rather than for each update, so use a defer.
	updated := make(map[int64]*Datacenter, len(t.updates))
	defer func() {
		for _, dc := range t.current {
			if u, ok := updated[dc.Id]; ok {
				dc.Description = u.Description
				dc.State = u.State
			}
		}
	}()
	for len(t.updates) > 0 {
		dc := t.updates[0]
		if _, err := stmt.ExecContext(c, dc.Description, dc.State, dc.Id); err != nil {
			return errors.Annotate(err, "failed to update datacenter %q", dc.Name).Err()
		}
		updated[dc.Id] = dc
		t.updates = t.updates[1:]
		logging.Infof(c, "Updated datacenter %q", dc.Name)
	}
	return nil
}

// ids returns a map of datacenter names to IDs.
func (t *DatacentersTable) ids(c context.Context) map[string]int64 {
	dcs := make(map[string]int64, len(t.current))
	for _, dc := range t.current {
		dcs[dc.Name] = dc.Id
	}
	return dcs
}

// EnsureDatacenters ensures the database contains exactly the given datacenters.
// Returns a map of datacenter names to IDs in the database.
func EnsureDatacenters(c context.Context, cfgs []*config.Datacenter) (map[string]int64, error) {
	t := &DatacentersTable{}
	if err := t.fetch(c); err != nil {
		return nil, errors.Annotate(err, "failed to fetch datacenters").Err()
	}
	t.computeChanges(c, cfgs)
	if err := t.add(c); err != nil {
		return nil, errors.Annotate(err, "failed to add datacenters").Err()
	}
	if err := t.remove(c); err != nil {
		return nil, errors.Annotate(err, "failed to remove datacenters").Err()
	}
	if err := t.update(c); err != nil {
		return nil, errors.Annotate(err, "failed to update datacenters").Err()
	}
	return t.ids(c), nil
}
