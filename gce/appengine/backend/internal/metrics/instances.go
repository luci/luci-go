// Copyright 2019 The LUCI Authors.
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

package metrics

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/gae/service/datastore"
)

var (
	configuredInstances = metric.NewInt(
		"gce/instances/configured",
		"The number of GCE instances configured to exist.",
		nil,
		field.String("prefix"),
		field.String("project"),
		field.String("resource_group"),
	)

	createdInstances = metric.NewInt(
		"gce/instances/created",
		"The number of GCE instances created.",
		nil,
		field.String("prefix"),
		field.String("project"),
		field.String("zone"),
	)

	CreatedInstanceChecked = metric.NewCounter(
		"gce/instance/creation/checked",
		"Cumulative number of GCE instances created and checked.",
		nil,
		field.String("prefix"),
		field.String("project"),
		field.String("zone"),
		field.String("instance"),
	)

	DestroyInstanceUnchecked = metric.NewCounter(
		"gce/instance/destroy/unchecked",
		"Cumulative number of GCE instances requested to destroy.",
		nil,
		field.String("prefix"),
		field.String("project"),
		field.String("zone"),
		field.String("instance"),
	)

	connectedInstances = metric.NewInt(
		"gce/instances/connected",
		"The number of GCE instances connected to Swarming.",
		nil,
		field.String("prefix"),
		field.String("project"),
		field.String("resource_group"),
		field.String("server"),
		field.String("zone"),
	)
)

// configuredCount encapsulates a count of configured VMs.
type configuredCount struct {
	Count   int
	Project string
}

// createdCount encapsulates a count of created VMs.
type createdCount struct {
	Count   int
	Project string
	Zone    string
}

// connectedCount encapsulates a count of connected VMs.
type connectedCount struct {
	Count   int
	Project string
	Server  string
	Zone    string
}

// InstanceCount is a root entity representing a count for instances with a
// common prefix.
type InstanceCount struct {
	// _extra is where unknown properties are put into memory.
	// Extra properties are not written to the datastore.
	_extra datastore.PropertyMap `gae:"-,extra"`
	// _kind is the entity's kind in the datastore.
	_kind string `gae:"$kind,InstanceCount"`
	// ID is the unique identifier for this count.
	ID string `gae:"$id"`
	// Prefix is the prefix for this count.
	Prefix string `gae:"prefix"`
	// ResourceGroup is the resource group for this prefix.
	ResourceGroup string `gae:"resource_group"`
	// Computed is the time this count was computed.
	Computed time.Time `gae:"computed"`
	// Configured is a slice of configuredCounts.
	Configured []configuredCount `gae:"configured,noindex"`
	// Connected is a slice of connectedCounts.
	Connected []connectedCount `gae:"connected,noindex"`
	// Created is a slice of createdCounts.
	Created []createdCount `gae:"created,noindex"`
}

// AddConfigured increments the count of configured VMs for the given project.
func (ic *InstanceCount) AddConfigured(n int, project string) {
	for i, c := range ic.Configured {
		if c.Project == project {
			ic.Configured[i].Count += n
			return
		}
	}
	ic.Configured = append(ic.Configured, configuredCount{
		Count:   n,
		Project: project,
	})
}

// AddCreated increments the count of created VMs for the given project and
// zone.
func (ic *InstanceCount) AddCreated(n int, project, zone string) {
	for i, c := range ic.Created {
		if c.Project == project && c.Zone == zone {
			ic.Created[i].Count += n
			return
		}
	}
	ic.Created = append(ic.Created, createdCount{
		Count:   n,
		Project: project,
		Zone:    zone,
	})
}

// AddConnected increments the count of connected VMs for the given project,
// server, and zone.
func (ic *InstanceCount) AddConnected(n int, project, server, zone string) {
	for i, c := range ic.Connected {
		if c.Project == project && c.Server == server && c.Zone == zone {
			ic.Connected[i].Count += n
			return
		}
	}
	ic.Connected = append(ic.Connected, connectedCount{
		Count:   n,
		Project: project,
		Server:  server,
		Zone:    zone,
	})
}

// Update updates metrics for all known counts of VMs for the given prefix.
func (ic *InstanceCount) Update(c context.Context, prefix, resourceGroup string) error {
	// Prefixes are globally unique, so we can use them as IDs.
	ic.ID = prefix
	ic.Computed = clock.Now(c).UTC()
	ic.Prefix = prefix
	ic.ResourceGroup = resourceGroup
	if err := datastore.Put(c, ic); err != nil {
		return errors.Annotate(err, "failed to store count").Err()
	}
	return nil
}

// updateInstances sets GCE instance metrics.
func updateInstances(c context.Context) {
	now := clock.Now(c)
	q := datastore.NewQuery("InstanceCount").Order("computed")
	if err := datastore.Run(c, q, func(ic *InstanceCount) {
		if now.Sub(ic.Computed) > 10*time.Minute {
			logging.Debugf(c, "deleting outdated count %q", ic.Prefix)
			if err := datastore.Delete(c, ic); err != nil {
				logging.Errorf(c, "%s", err)
			}
			return
		}
		for _, conf := range ic.Configured {
			configuredInstances.Set(c, int64(conf.Count), ic.Prefix, conf.Project, ic.ResourceGroup)
		}
		for _, crea := range ic.Created {
			createdInstances.Set(c, int64(crea.Count), ic.Prefix, crea.Project, crea.Zone)
		}
		for _, conn := range ic.Connected {
			connectedInstances.Set(c, int64(conn.Count), ic.Prefix, conn.Project, ic.ResourceGroup, conn.Server, conn.Zone)
		}
	}); err != nil {
		errors.Log(c, errors.Annotate(err, "failed to fetch counts").Err())
	}
}

func init() {
	tsmon.RegisterGlobalCallback(updateInstances, configuredInstances, connectedInstances, createdInstances)
}
