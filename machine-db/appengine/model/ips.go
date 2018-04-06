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
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/machine-db/common"
)

// IP represents a row in the ips table.
type IP struct {
	Id     int64
	IPv4   common.IPv4
	VLANId int64
}

// IPsTable represents the table of IP addresses in the database.
type IPsTable struct {
	// current is the slice of IP addresses in the database.
	current []*IP

	// additions is a slice of IP addresses pending addition to the database.
	additions []*IP
	// removals is a slice of IP addresses pending removal from the database.
	removals []*IP
	// updates is a slice of IP addresses pending update in the database.
	updates []*IP
}

// fetch fetches the IP addresses from the database.
func (t *IPsTable) fetch(c context.Context) error {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT id, ipv4, vlan_id
		FROM ips
	`)
	if err != nil {
		return errors.Annotate(err, "failed to select IP addresses").Err()
	}
	defer rows.Close()
	for rows.Next() {
		ip := &IP{}
		if err := rows.Scan(&ip.Id, &ip.IPv4, &ip.VLANId); err != nil {
			return errors.Annotate(err, "failed to scan IP address").Err()
		}
		t.current = append(t.current, ip)
	}
	return nil
}

// needsUpdate returns true if the given row needs to be updated to match the given config.
func (*IPsTable) needsUpdate(row, cfg *IP) bool {
	return row.VLANId != cfg.VLANId
}

// computeChanges computes the changes that need to be made to the IP addresses in the database.
func (t *IPsTable) computeChanges(c context.Context, vlans []*config.VLAN) error {
	cfgs := make(map[common.IPv4]*IP, len(vlans))
	for _, vlan := range vlans {
		ipv4, length, err := common.IPv4Range(vlan.CidrBlock)
		if err != nil {
			return errors.Annotate(err, "failed to determine start address and range for CIDR block %q", vlan.CidrBlock).Err()
		}
		// TODO(smut): Mark the first address as the network address and the last address as the broadcast address.
		for i := int64(0); i < length; i++ {
			cfgs[ipv4] = &IP{
				IPv4:   ipv4,
				VLANId: vlan.Id,
			}
			ipv4 += 1
		}
	}

	for _, ip := range t.current {
		if cfg, ok := cfgs[ip.IPv4]; ok {
			// IP address found in the config.
			if t.needsUpdate(ip, cfg) {
				// IP address doesn't match the config.
				cfg.Id = ip.Id
				t.updates = append(t.updates, cfg)
			}
			// Record that the IP address config has been seen.
			delete(cfgs, cfg.IPv4)
		} else {
			// IP address not found in the config.
			t.removals = append(t.removals, ip)
		}
	}

	// IP addresses remaining in the map are present in the config but not the database.
	// Iterate deterministically over the slices to determine which IP addresses need to be added.
	for _, vlan := range vlans {
		ipv4, length, err := common.IPv4Range(vlan.CidrBlock)
		if err != nil {
			return errors.Annotate(err, "failed to determine start address and range for CIDR block %q", vlan.CidrBlock).Err()
		}
		for i := int64(0); i < length; i++ {
			if ip, ok := cfgs[ipv4]; ok {
				t.additions = append(t.additions, ip)
			}
			ipv4 += 1
		}
	}
	return nil
}

// add adds all IP addresses pending addition to the database, clearing pending additions.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *IPsTable) add(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.additions) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		INSERT INTO ips (ipv4, vlan_id)
		VALUES (?, ?)
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Add each IP address to the database, and update the slice of IP address with each addition.
	// TODO(smut): Batch SQL operations if it becomes necessary.
	logging.Infof(c, "Attempting to add %d IP addresses", len(t.additions))
	for len(t.additions) > 0 {
		ip := t.additions[0]
		result, err := stmt.ExecContext(c, ip.IPv4, ip.VLANId)
		if err != nil {
			return errors.Annotate(err, "failed to add IP address %q", ip.IPv4).Err()
		}
		t.current = append(t.current, ip)
		t.additions = t.additions[1:]
		logging.Infof(c, "Added IP address %q", ip.IPv4)
		ip.Id, err = result.LastInsertId()
		if err != nil {
			return errors.Annotate(err, "failed to get IP address ID %q", ip.IPv4).Err()
		}
	}
	return nil
}

// remove removes all IP addresses pending removal from the database, clearing pending removals.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *IPsTable) remove(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.removals) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		DELETE FROM ips
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Remove each IP address from the table. It's more efficient to update the slice of
	// IP addresses once at the end rather than for each removal, so use a defer.
	removed := make(map[int64]struct{}, len(t.removals))
	defer func() {
		var ips []*IP
		for _, ip := range t.current {
			if _, ok := removed[ip.Id]; !ok {
				ips = append(ips, ip)
			}
		}
		t.current = ips
	}()
	for len(t.removals) > 0 {
		ip := t.removals[0]
		if _, err := stmt.ExecContext(c, ip.Id); err != nil {
			// Defer ensures the slice of IP addresses is updated even if we exit early.
			return errors.Annotate(err, "failed to remove IP address %q", ip.IPv4).Err()
		}
		removed[ip.Id] = struct{}{}
		t.removals = t.removals[1:]
		logging.Infof(c, "Removed IP address %q", ip.IPv4)
	}
	return nil
}

// update updates all IP addresses pending update in the database, clearing pending updates.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *IPsTable) update(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.updates) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		UPDATE ips
		SET vlan_id = ?
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Update each IP address in the table. It's more efficient to update the slice of
	// IP addresses once at the end rather than for each update, so use a defer.
	updated := make(map[int64]*IP, len(t.updates))
	defer func() {
		for _, ip := range t.current {
			if u, ok := updated[ip.Id]; ok {
				ip.VLANId = u.VLANId
			}
		}
	}()
	for len(t.updates) > 0 {
		ip := t.updates[0]
		if _, err := stmt.ExecContext(c, ip.VLANId, ip.Id); err != nil {
			return errors.Annotate(err, "failed to update IP address %q", ip.IPv4).Err()
		}
		updated[ip.Id] = ip
		t.updates = t.updates[1:]
		logging.Infof(c, "Updated IP address %q", ip.IPv4)
	}
	return nil
}

// ids returns a map of IP addresses to IDs.
func (t *IPsTable) ids(c context.Context) map[common.IPv4]int64 {
	ips := make(map[common.IPv4]int64, len(t.current))
	for _, ip := range t.current {
		ips[ip.IPv4] = ip.Id
	}
	return ips
}

// EnsureIPs ensures the database contains exactly the given IP addresses.
func EnsureIPs(c context.Context, cfgs []*config.VLAN) error {
	t := &IPsTable{}
	if err := t.fetch(c); err != nil {
		return errors.Annotate(err, "failed to fetch IP addresses").Err()
	}
	if err := t.computeChanges(c, cfgs); err != nil {
		return errors.Annotate(err, "failed to compute changes").Err()
	}
	if err := t.add(c); err != nil {
		return errors.Annotate(err, "failed to add IP addresses").Err()
	}
	if err := t.remove(c); err != nil {
		return errors.Annotate(err, "failed to remove IP addresses").Err()
	}
	if err := t.update(c); err != nil {
		return errors.Annotate(err, "failed to update IP addresses").Err()
	}
	return nil
}
