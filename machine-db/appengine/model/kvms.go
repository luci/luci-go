// Copyright 2018 The LUCI Authors.
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
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/machine-db/common"
)

// KVM represents a row in the kvms table.
type KVM struct {
	config.KVM
	IPv4       common.IPv4
	MacAddress common.MAC48
	PlatformId int64
	RackId     int64
	Id         int64
	HostnameId int64
}

// KVMsTable represents the table of KVMs in the database.
type KVMsTable struct {
	// platforms is a map of platform name to ID in the database.
	platforms map[string]int64
	// racks is a map of rack name to ID in the database.
	racks map[string]int64
	// current is the slice of KVMs in the database.
	current []*KVM

	// additions is a slice of KVMs pending addition to the database.
	additions []*KVM
	// removals is a slice of KVMs pending removal from the database.
	removals []*KVM
	// updates is a slice of KVMs pending update in the database.
	updates []*KVM
}

// fetch fetches the KVMs from the database.
func (t *KVMsTable) fetch(c context.Context) error {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT k.id, h.id, h.name, i.ipv4, k.description, k.mac_address, k.state, k.platform_id, k.rack_id
		FROM kvms k, hostnames h, ips i
		WHERE k.hostname_id = h.id AND i.hostname_id = h.id
	`)
	if err != nil {
		return errors.Annotate(err, "failed to select KVMs").Err()
	}
	defer rows.Close()
	for rows.Next() {
		kvm := &KVM{}
		if err := rows.Scan(&kvm.Id, &kvm.HostnameId, &kvm.Name, &kvm.IPv4, &kvm.Description, &kvm.MacAddress, &kvm.State, &kvm.PlatformId, &kvm.RackId); err != nil {
			return errors.Annotate(err, "failed to scan KVM").Err()
		}
		t.current = append(t.current, kvm)
	}
	return nil
}

// needsUpdate returns true if the given row needs to be updated to match the given config.
func (*KVMsTable) needsUpdate(row, cfg *KVM) bool {
	// TODO(smut): Update IP address.
	return row.MacAddress != cfg.MacAddress || row.Description != cfg.Description || row.State != cfg.State || row.PlatformId != cfg.PlatformId || row.RackId != cfg.RackId
}

// computeChanges computes the changes that need to be made to the KVMs in the database.
func (t *KVMsTable) computeChanges(c context.Context, datacenters []*config.Datacenter) error {
	cfgs := make(map[string]*KVM, len(datacenters))
	for _, dc := range datacenters {
		for _, cfg := range dc.Kvm {
			ipv4, err := common.ParseIPv4(cfg.Ipv4)
			if err != nil {
				return errors.Reason("failed to determine IP address for KVM %q: invalid IPv4 address %q", cfg.Name, cfg.Ipv4).Err()
			}
			mac48, err := common.ParseMAC48(cfg.MacAddress)
			if err != nil {
				return errors.Reason("failed to determine MAC address for KVM %q: invalid MAC-48 address %q", cfg.Name, cfg.MacAddress).Err()
			}
			pId, ok := t.platforms[cfg.Platform]
			if !ok {
				return errors.Reason("failed to determine platform ID for KVM %q: platform %q does not exist", cfg.Name, cfg.Platform).Err()
			}
			rId, ok := t.racks[cfg.Rack]
			if !ok {
				return errors.Reason("failed to determine rack ID for KVM %q: rack %q does not exist", cfg.Name, cfg.Rack).Err()
			}
			cfgs[cfg.Name] = &KVM{
				KVM: config.KVM{
					Name:        cfg.Name,
					Description: cfg.Description,
					State:       cfg.State,
				},
				IPv4:       ipv4,
				MacAddress: mac48,
				PlatformId: pId,
				RackId:     rId,
			}
		}
	}

	for _, kvm := range t.current {
		if cfg, ok := cfgs[kvm.Name]; ok {
			// KVM found in the config.
			if t.needsUpdate(kvm, cfg) {
				// KVM doesn't match the config.
				cfg.Id = kvm.Id
				t.updates = append(t.updates, cfg)
			}
			// Record that the KVM config has been seen.
			delete(cfgs, cfg.Name)
		} else {
			// KVM not found in the config.
			t.removals = append(t.removals, kvm)
		}
	}

	// KVMs remaining in the map are present in the config but not the database.
	// Iterate deterministically over the slices to determine which KVMs need to be added.
	for _, dc := range datacenters {
		for _, cfg := range dc.Kvm {
			if kvm, ok := cfgs[cfg.Name]; ok {
				t.additions = append(t.additions, kvm)
			}
		}
	}
	return nil
}

// add adds all KVMs pending addition to the database, clearing pending additions.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *KVMsTable) add(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.additions) == 0 {
		return nil
	}

	tx, err := database.Begin(c)
	if err != nil {
		return errors.Annotate(err, "failed to begin transaction").Err()
	}
	defer tx.MaybeRollback(c)
	stmt, err := tx.PrepareContext(c, `
		INSERT INTO kvms (hostname_id, description, mac_address, state, platform_id, rack_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Add each KVM to the database, and update the slice of KVMs with each addition.
	for len(t.additions) > 0 {
		kvm := t.additions[0]
		hostnameId, err := AssignHostnameAndIP(c, tx, kvm.Name, kvm.IPv4)
		if err != nil {
			return errors.Annotate(err, "failed to add KVM %q", kvm.Name).Err()
		}
		result, err := stmt.ExecContext(c, hostnameId, kvm.Description, kvm.MacAddress, kvm.State, kvm.PlatformId, kvm.RackId)
		if err != nil {
			return errors.Annotate(err, "failed to add KVM %q", kvm.Name).Err()
		}
		t.current = append(t.current, kvm)
		t.additions = t.additions[1:]
		logging.Infof(c, "Added KVM %q", kvm.Name)
		kvm.Id, err = result.LastInsertId()
		if err != nil {
			return errors.Annotate(err, "failed to get KVM ID %q", kvm.Name).Err()
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Annotate(err, "failed to commit transaction").Err()
	}
	return nil
}

// remove removes all KVMs pending removal from the database, clearing pending removals.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *KVMsTable) remove(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.removals) == 0 {
		return nil
	}

	db := database.Get(c)
	// By setting kvms.hostname_id ON DELETE CASCADE when setting up the database, we can avoid
	// having to explicitly delete the KVM here. MySQL will cascade the deletion to the KVM.
	stmt, err := db.PrepareContext(c, `
		DELETE FROM hostnames
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Remove each KVM from the table. It's more efficient to update the slice of
	// KVMs once at the end rather than for each removal, so use a defer.
	removed := make(map[int64]struct{}, len(t.removals))
	defer func() {
		var kvms []*KVM
		for _, kvm := range t.current {
			if _, ok := removed[kvm.Id]; !ok {
				kvms = append(kvms, kvm)
			}
		}
		t.current = kvms
	}()
	for len(t.removals) > 0 {
		kvm := t.removals[0]
		if _, err := stmt.ExecContext(c, kvm.HostnameId); err != nil {
			// Defer ensures the slice of KVMs is updated even if we exit early.
			return errors.Annotate(err, "failed to remove KVM %q", kvm.Name).Err()
		}
		removed[kvm.Id] = struct{}{}
		t.removals = t.removals[1:]
		logging.Infof(c, "Removed KVM %q", kvm.Name)
	}
	return nil
}

// update updates all KVMs pending update in the database, clearing pending updates.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *KVMsTable) update(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.updates) == 0 {
		return nil
	}

	db := database.Get(c)
	// TODO(smut): Update IP address.
	stmt, err := db.PrepareContext(c, `
		UPDATE kvms
		SET description = ?, mac_address = ?, state = ?, platform_id = ?, rack_id = ?
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Update each KVM in the table. It's more efficient to update the slice of
	// KVMs once at the end rather than for each update, so use a defer.
	updated := make(map[int64]*KVM, len(t.updates))
	defer func() {
		for _, kvm := range t.current {
			if u, ok := updated[kvm.Id]; ok {
				kvm.MacAddress = u.MacAddress
				kvm.Description = u.Description
				kvm.State = u.State
				kvm.PlatformId = u.PlatformId
				kvm.RackId = u.RackId
			}
		}
	}()
	for len(t.updates) > 0 {
		kvm := t.updates[0]
		if _, err := stmt.ExecContext(c, kvm.Description, kvm.MacAddress, kvm.State, kvm.PlatformId, kvm.RackId, kvm.Id); err != nil {
			return errors.Annotate(err, "failed to update KVM %q", kvm.Name).Err()
		}
		updated[kvm.Id] = kvm
		t.updates = t.updates[1:]
		logging.Infof(c, "Updated KVM %q", kvm.Name)
	}
	return nil
}

// ids returns a map of KVM names to IDs.
func (t *KVMsTable) ids(c context.Context) map[string]int64 {
	KVMs := make(map[string]int64, len(t.current))
	for _, s := range t.current {
		KVMs[s.Name] = s.Id
	}
	return KVMs
}

// EnsureKVMs ensures the database contains exactly the given KVMs.
// Returns a map of KVM names to IDs in the database.
func EnsureKVMs(c context.Context, cfgs []*config.Datacenter, platformIds, rackIds map[string]int64) (map[string]int64, error) {
	t := &KVMsTable{}
	t.platforms = platformIds
	t.racks = rackIds
	if err := t.fetch(c); err != nil {
		return nil, errors.Annotate(err, "failed to fetch KVMs").Err()
	}
	if err := t.computeChanges(c, cfgs); err != nil {
		return nil, errors.Annotate(err, "failed to compute changes").Err()
	}
	if err := t.add(c); err != nil {
		return nil, errors.Annotate(err, "failed to add KVMs").Err()
	}
	if err := t.remove(c); err != nil {
		return nil, errors.Annotate(err, "failed to remove KVMs").Err()
	}
	if err := t.update(c); err != nil {
		return nil, errors.Annotate(err, "failed to update KVMs").Err()
	}
	return t.ids(c), nil
}
