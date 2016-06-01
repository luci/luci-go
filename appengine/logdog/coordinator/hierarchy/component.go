// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package hierarchy

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/logdog/types"
)

// Component is a log stream hierarchy component.
type Component struct {
	// Parent is the partial stream path parent of this component.
	Parent types.StreamPath
	// Name is the name of this Component.
	Name string
	// Stream is true if this is a stream component, false if it is a path
	// component.
	Stream bool
}

// Components returns the set of hierarchy components for a given stream path.
func Components(p types.StreamPath) []*Component {
	// Build all Component objects for our parts.
	//
	// The first component is the stream component.
	components := make([]*Component, 0, p.SegmentCount())
	for {
		var last string
		p, last = p.SplitLast()
		components = append(components, &Component{p, last, len(components) == 0})

		if p == "" {
			break
		}
	}

	return components
}

// String returns a string representation for this Component.
//
// For path components, this is the path. For stream components, this is the
// path followed by a dollar sign. Note that the dollar sign is not a valid
// types.StreamName character, and so this will not conflict with other valid
// Components.
//
// For example, for {Parent="foo/bar", Name="baz"}, we get "foo/bar/baz".
// If {Stream=true}, we would get "foo/bar/baz$".
//
// Two Components with the same String result reference the same path component.
func (c *Component) String() string {
	s := string(c.Path())
	if c.Stream {
		s += "$"
	}
	return s
}

// Path returns the StreamPath for this Component.
func (c *Component) Path() types.StreamPath {
	return c.Parent.Append(c.Name)
}

// Exists checks whether this Component exists in the datastore.
func (c *Component) Exists(di ds.Interface) (bool, error) {
	return di.Exists(di.KeyForObj(c.entity()))
}

// Put writes this Component to the datastore.
//
// Prior to writing, it will verify that the Component represents a valid
// partial log stream path.
func (c *Component) Put(di ds.Interface) error {
	path := c.Path()
	if err := path.ValidatePartial(); err != nil {
		return err
	}
	return di.Put(c.entity())
}

// entity returns the componentEntity that this Component describes.
func (c *Component) entity() *componentEntity {
	return mkComponentEntity(c.Parent, c.Name, c.Stream)
}

// Missing checks the status of the supplied Components in the datastore.
//
// It mutates the supplied components array, shrinking it if necessary, to
// return only the Components that were not already present.
//
// If Missing failed, it will return the original components array and the
// failure error.
func Missing(di ds.Interface, components []*Component) ([]*Component, error) {
	exists := make([]*ds.Key, len(components))
	for i, c := range components {
		exists[i] = di.KeyForObj(c.entity())
	}
	bl, err := di.ExistsMulti(exists)
	if err != nil {
		return nil, err
	}

	// Condense the components array.
	nidx := 0
	for i, b := range bl {
		if !b {
			components[nidx] = components[i]
			nidx++
		}
	}
	return components[:nidx], nil
}

// PutMulti performs a datastore PutMulti on all of the hierarchy entities for
// components.
func PutMulti(di ds.Interface, components []*Component) error {
	ents := make([]*componentEntity, len(components))
	for i, comp := range components {
		ents[i] = comp.entity()
	}
	return di.PutMulti(ents)
}
