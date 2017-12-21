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

// VLAN represents a vlan.
type VLAN struct {
	config.VLAN
}

// VLANsTable represents the table of vlans in the database.
type VLANsTable struct {
	// current is the slice of vlans in the database.
	current []*VLAN

	// additions is a slice of vlans pending addition to the database.
	additions []*VLAN
	// removals is a slice of vlans pending removal from the database.
	removals []*VLAN
	// updates is a slice of vlans pending update in the database.
	updates []*VLAN
}

// fetch fetches the vlans from the database.
func (t *VLANsTable) fetch(c context.Context) error {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT id, alias
		FROM vlans
	`)
	if err != nil {
		return errors.Annotate(err, "failed to select vlans").Err()
	}
	defer rows.Close()
	for rows.Next() {
		vlan := &VLAN{}
		if err := rows.Scan(&vlan.Id, &vlan.Alias); err != nil {
			return errors.Annotate(err, "failed to scan vlan").Err()
		}
		t.current = append(t.current, vlan)
	}
	return nil
}

// needsUpdate returns true if the given row needs to be updated to match the given config.
func (*VLANsTable) needsUpdate(row, cfg *VLAN) bool {
	return row.Alias != cfg.Alias
}

// computeChanges computes the changes that need to be made to the vlans in the database.
func (t *VLANsTable) computeChanges(c context.Context, vlans []*config.VLAN) {
	cfgs := make(map[int64]*VLAN, len(vlans))
	for _, cfg := range vlans {
		cfgs[cfg.Id] = &VLAN{
			VLAN: config.VLAN{
				Id:    cfg.Id,
				Alias: cfg.Alias,
			},
		}
	}

	for _, vlan := range t.current {
		if cfg, ok := cfgs[vlan.Id]; ok {
			// Vlan found in the config.
			if t.needsUpdate(vlan, cfg) {
				// Vlan doesn't match the config.
				t.updates = append(t.updates, cfg)
			}
			// Record that the vlan config has been seen.
			delete(cfgs, cfg.Id)
		} else {
			// Vlan not found in the config.
			t.removals = append(t.removals, vlan)
		}
	}

	// Vlans remaining in the map are present in the config but not the database.
	// Iterate deterministically over the slice to determine which vlans need to be added.
	for _, cfg := range vlans {
		if p, ok := cfgs[cfg.Id]; ok {
			t.additions = append(t.additions, p)
		}
	}
}

// add adds all vlans pending addition to the database, clearing pending additions.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *VLANsTable) add(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.additions) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		INSERT INTO vlans (id, alias)
		VALUES (?, ?)
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Add each vlan to the database, and update the slice of vlans with each addition.
	for len(t.additions) > 0 {
		vlan := t.additions[0]
		_, err := stmt.ExecContext(c, vlan.Id, vlan.Alias)
		if err != nil {
			return errors.Annotate(err, "failed to add vlan %d", vlan.Id).Err()
		}
		t.current = append(t.current, vlan)
		t.additions = t.additions[1:]
		logging.Infof(c, "Added vlan %d", vlan.Id)
	}
	return nil
}

// remove removes all vlans pending removal from the database, clearing pending removals.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *VLANsTable) remove(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.removals) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		DELETE FROM vlans
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Remove each vlans from the database. It's more efficient to update the slice of
	// vlans once at the end rather than for each removal, so use a defer.
	removed := make(map[int64]struct{}, len(t.removals))
	defer func() {
		var vlans []*VLAN
		for _, vlan := range t.current {
			if _, ok := removed[vlan.Id]; !ok {
				vlans = append(vlans, vlan)
			}
		}
		t.current = vlans
	}()
	for len(t.removals) > 0 {
		vlan := t.removals[0]
		if _, err := stmt.ExecContext(c, vlan.Id); err != nil {
			// Defer ensures the slice of vlans is updated even if we exit early.
			return errors.Annotate(err, "failed to remove vlan %d", vlan.Id).Err()
		}
		removed[vlan.Id] = struct{}{}
		t.removals = t.removals[1:]
		logging.Infof(c, "Removed vlan %d", vlan.Id)
	}
	return nil
}

// update updates all vlans pending update in the database, clearing pending updates.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *VLANsTable) update(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.updates) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		UPDATE vlans
		SET alias = ?
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Update each vlan in the database. It's more efficient to update the slice of
	// vlans once at the end rather than for each update, so use a defer.
	updated := make(map[int64]*VLAN, len(t.updates))
	defer func() {
		for _, vlan := range t.current {
			if u, ok := updated[vlan.Id]; ok {
				vlan.Alias = u.Alias
			}
		}
	}()
	for len(t.updates) > 0 {
		vlan := t.updates[0]
		if _, err := stmt.ExecContext(c, vlan.Alias, vlan.Id); err != nil {
			return errors.Annotate(err, "failed to update vlan %d", vlan.Id).Err()
		}
		updated[vlan.Id] = vlan
		t.updates = t.updates[1:]
		logging.Infof(c, "Updated vlan %d", vlan.Id)
	}
	return nil
}

// EnsureVLANs ensures the database contains exactly the given vlans.
func EnsureVLANs(c context.Context, cfgs []*config.VLAN) error {
	t := &VLANsTable{}
	if err := t.fetch(c); err != nil {
		return errors.Annotate(err, "failed to fetch vlans").Err()
	}
	t.computeChanges(c, cfgs)
	if err := t.add(c); err != nil {
		return errors.Annotate(err, "failed to add vlans").Err()
	}
	if err := t.remove(c); err != nil {
		return errors.Annotate(err, "failed to remove vlans").Err()
	}
	if err := t.update(c); err != nil {
		return errors.Annotate(err, "failed to update vlans").Err()
	}
	return nil
}
