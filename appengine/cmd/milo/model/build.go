// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/parallel"
	"golang.org/x/net/context"
)

// BuildRoot is the entity group root of a set of builds. Currently, we group
// builds in a master/builder style hierarchy, emulating buildbot.
type BuildRoot struct {
	*datastore.Key
}

// GetBuildRoot returns the BuildRoot for the corresponding master and
// builder combination.
// Key format is "schema:<arbitrary text>".
func GetBuildRoot(c context.Context, master, builder string) BuildRoot {
	ds := datastore.Get(c)
	return BuildRoot{
		ds.NewKey("BuildRoot", fmt.Sprintf("buildbot:%s|%s", master, builder), 0, nil),
	}
}

// GetBuilds returns a query which represents all the Builds this BuildRoot
// cares about.
func (b *BuildRoot) GetBuilds() *datastore.Query {
	return datastore.NewQuery("Build").Ancestor(b.Key).Order("-revisions.generation")
}

// GetBuilds retrieves a list of the most "recent" (as determined by
// generation number) n builds for each BuildRoot in parallel.
func GetBuilds(c context.Context, sources []BuildRoot, n int32) ([][]*Build, error) {
	ds := datastore.Get(c)

	rootBuilds := make([][]*Build, len(sources))
	err := parallel.WorkPool(10, func(ch chan<- func() error) {
		for i, root := range sources {
			i := i
			root := root
			ch <- func() error {
				// TODO(martiniss) add caching
				q := root.GetBuilds().Limit(n)

				rootBuilds[i] = make([]*Build, 0, n)
				err := ds.GetAll(q, &rootBuilds[i])

				return err
			}
		}
	})

	return rootBuilds, err
}

// RevisionInfo is a small struct used to refer to a particular Revision by a
// Build. Its main use is allowing Builds to be sorted by generation number.
type RevisionInfo struct {
	Repository string `gae:"repo"`
	Digest     string `gae:"hash,noindex"`
	Generation int    `gae:"generation"`
}

// GetKey returns the corresponding Revision for this RevisionInfo.
func (r *RevisionInfo) GetKey(c context.Context) *datastore.Key {
	ds := datastore.Get(c)
	return ds.MakeKey("Revision", r.Digest, 0, GetRepository(c, r.Repository))
}

// Build represents an execution of some code at specific revision(s), which
// milo caches for display to end-users.
type Build struct {
	// ExecutionTime is the time at which this build was executed.
	ExecutionTime TimeID `gae:"$id"`

	// BuildRoot is the key of the BuildRoot this Build belongs to.
	BuildRoot *datastore.Key `gae:"$parent"`

	// Revisions is all of the Revisions that are relevant to this Build.
	Revisions []RevisionInfo `gae:"revisions"`

	// BuildLogKey is needed to look up the build log for this Build. Currently,
	// this is the swarming task ID.
	BuildLogKey string

	// InfraStatus is the status of the Build as determined by the scheduler.
	InfraStatus string

	// UserStatus is the status of the Build as determined by the log source.
	UserStatus string
}

// TimeID is a time.Time which can be embedded in a datastore entity as
// a millisecond timestamp.
type TimeID struct {
	time.Time
}

var _ datastore.PropertyConverter = (*TimeID)(nil)

// FromProperty converts a raw datastore Property into a TimeID.
func (t *TimeID) FromProperty(p datastore.Property) error {
	negVal, err := p.Project(datastore.PTInt) // may be a string
	if err != nil {
		return err
	}

	if err = p.SetValue(-negVal.(int64), true); err != nil {
		return err
	}
	tVal, err := p.Project(datastore.PTTime)
	if err != nil {
		return err
	}
	t.Time = tVal.(time.Time)
	return nil
}

// ToProperty converts a TimeID to a raw datastore Property.
func (t *TimeID) ToProperty() (ret datastore.Property, err error) {
	tProp := datastore.MkProperty(t.Time)

	negInt, err := tProp.Project(datastore.PTInt)
	if err != nil {
		return
	}

	err = ret.SetValue(-negInt.(int64), true)
	return
}
