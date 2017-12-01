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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// Datacenter represents a datacenter.
type Datacenter struct {
	config.DatacenterConfig
	Id int64
}

// DatacentersTable represents the table of datacenters in the database.
type DatacentersTable struct {
	// current is the slice of datacenters in the database.
	current []*Datacenter

	// additions is a slice of datacenters pending addition to the database.
	additions []*config.DatacenterConfig
	// removals is a slice of names of datacenters pending removal from the database.
	removals []string
	// updates is a slice of datacenters pending update in the database.
	updates []*Datacenter
}

// fetch fetches the datacenters from the database.
func (d *DatacentersTable) fetch(c context.Context) error {
	db := database.Get(c)
	rows, err := db.QueryContext(c, "SELECT id, name, description FROM datacenters")
	if err != nil {
		return errors.Annotate(err, "failed to select datacenters").Err()
	}
	defer rows.Close()
	for rows.Next() {
		dc := &Datacenter{}
		if err := rows.Scan(&dc.Id, &dc.Name, &dc.Description); err != nil {
			return errors.Annotate(err, "failed to scan datacenter").Err()
		}
		d.current = append(d.current, dc)
	}
	return nil
}

// computeChanges computes the changes that need to be made to the datacenters in the database.
func (d *DatacentersTable) computeChanges(c context.Context, datacenters []*config.DatacenterConfig) {
	cfgs := make(map[string]*config.DatacenterConfig, len(datacenters))
	for _, cfg := range datacenters {
		cfgs[cfg.Name] = cfg
	}

	for _, dc := range d.current {
		if cfg, ok := cfgs[dc.Name]; ok {
			// Datacenter found in the config.
			if dc.Description != cfg.Description {
				// Datacenter doesn't match the config.
				d.updates = append(d.updates, &Datacenter{
					DatacenterConfig: config.DatacenterConfig{
						Name:        cfg.Name,
						Description: cfg.Description,
					},
					Id: dc.Id,
				})
			}
			// Record that the datacenter config has been seen.
			delete(cfgs, cfg.Name)
		} else {
			// Datacenter not found in the config.
			d.removals = append(d.removals, dc.Name)
		}
	}

	// Datacenters remaining in the map are present in the config but not the database.
	// Iterate deterministically over the slice to determine which datacenters need to be added.
	for _, cfg := range datacenters {
		if _, ok := cfgs[cfg.Name]; ok {
			d.additions = append(d.additions, cfg)
		}
	}
}

// add adds all datacenters pending addition to the database, clearing pending additions.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (d *DatacentersTable) add(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(d.additions) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, "INSERT INTO datacenters (name, description) VALUES (?, ?)")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Add each datacenter to the database, and update the slice of datacenters with each addition.
	for len(d.additions) > 0 {
		cfg := d.additions[0]
		result, err := stmt.ExecContext(c, cfg.Name, cfg.Description)
		if err != nil {
			return errors.Annotate(err, "failed to add datacenter: %s", cfg.Name).Err()
		}
		dc := &Datacenter{
			DatacenterConfig: config.DatacenterConfig{
				Name:        cfg.Name,
				Description: cfg.Description,
			},
		}
		d.current = append(d.current, dc)
		d.additions = d.additions[1:]
		logging.Infof(c, "Added datacenter: %s", dc.Name)
		dc.Id, err = result.LastInsertId()
		if err != nil {
			return errors.Annotate(err, "failed to get datacenter ID: %s", cfg.Name).Err()
		}
	}
	return nil
}

// remove removes all datacenters pending removal from the database, clearing pending removals.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (d *DatacentersTable) remove(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(d.removals) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, "DELETE FROM datacenters WHERE name = ?")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Remove each datacenter from the database. It's more efficient to update the slice of
	// datacenters once at the end rather than for each removal, so use a defer.
	removed := stringset.New(len(d.removals))
	defer func() {
		var dcs []*Datacenter
		for _, dc := range d.current {
			if !removed.Has(dc.Name) {
				dcs = append(dcs, dc)
			}
		}
		d.current = dcs
	}()
	for len(d.removals) > 0 {
		dc := d.removals[0]
		if _, err := stmt.ExecContext(c, dc); err != nil {
			// Defer ensures the slice of datacenters is updated even if we exit early.
			return errors.Annotate(err, "failed to remove datacenter: %s", dc).Err()
		}
		removed.Add(dc)
		d.removals = d.removals[1:]
		logging.Infof(c, "Removed datacenter: %s", dc)
	}
	return nil
}

// update updates all datacenters pending update in the database, clearing pending updates.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (d *DatacentersTable) update(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(d.updates) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, "UPDATE datacenters SET description = ? WHERE id = ?")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Update each datacenter in the database. It's more efficient to update the slice of
	// datacenters once at the end rather than for each update, so use a defer.
	updated := make(map[string]*Datacenter, len(d.updates))
	defer func() {
		for _, dc := range d.current {
			if _, ok := updated[dc.Name]; ok {
				dc.Description = updated[dc.Name].Description
			}
		}
	}()
	for len(d.updates) > 0 {
		dc := d.updates[0]
		if _, err := stmt.ExecContext(c, dc.Description, dc.Id); err != nil {
			return errors.Annotate(err, "failed to update datacenter: %s", dc.Name).Err()
		}
		updated[dc.Name] = dc
		d.updates = d.updates[1:]
		logging.Infof(c, "Updated datacenter: %s", dc.Name)
	}
	return nil
}

// ids returns a map of datacenter names to IDs.
func (d *DatacentersTable) ids(c context.Context) map[string]int64 {
	dcs := make(map[string]int64, len(d.current))
	for _, dc := range d.current {
		dcs[dc.Name] = dc.Id
	}
	return dcs
}

// EnsureDatacenters ensures the database contains exactly the given datacenters.
// Returns a map of datacenter names to IDs in the database.
func EnsureDatacenters(c context.Context, cfgs []*config.DatacenterConfig) (map[string]int64, error) {
	d := &DatacentersTable{}
	if err := d.fetch(c); err != nil {
		return nil, errors.Annotate(err, "failed to fetch datacenters").Err()
	}
	d.computeChanges(c, cfgs)
	if err := d.add(c); err != nil {
		return nil, errors.Annotate(err, "failed to add datacenters").Err()
	}
	if err := d.remove(c); err != nil {
		return nil, errors.Annotate(err, "failed to remove datacenters").Err()
	}
	if err := d.update(c); err != nil {
		return nil, errors.Annotate(err, "failed to update datacenters").Err()
	}
	return d.ids(c), nil
}
