// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hierarchy

import (
	"fmt"
	"strings"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/logdog/types"
)

// componentID is the datastore ID for a component.
//
// A component represents a single component, and is keyed on the
// component's value and whether the component is a stream or path component.
//
// A log stream path is broken into several components. For example,
// "foo/bar/+/baz/qux" is broken into ["foo", "bar", "+", "baz", "qux"]. The
// intermediate pieces, "foo", "bar", and "baz" are "path components" (e.g.,
// directory name). The terminating component, "qux", is the stream component
// (e.g., file names), since adding it generates a valid stream name.
//
// Note that two streams, "foo/+/bar" and "foo/+/bar/baz", result in components
// "bar". In the first, "bar" is a path component, and in the second "bar" is a
// stream component.
//
// Path component IDs have "~" prepended to them, since "~" is not a valid
// initial stream name character and "~" comes bytewise after all valid
// characters, this will cause paths to sort after streams for a given hierarchy
// level.
type componentID struct {
	// name is the name of this component.
	name string
	// stream is true if this component is a stream path, false if it is a path
	// component.
	stream bool
}

var _ ds.PropertyConverter = (*componentID)(nil)

// ToProperty implements ds.PropertyConverter
func (id *componentID) ToProperty() (ds.Property, error) {
	return ds.MkPropertyNI(id.id()), nil
}

// FromProperty implements ds.PropertyConverter
func (id *componentID) FromProperty(p ds.Property) error {
	if p.Type() != ds.PTString {
		return fmt.Errorf("wrong type for property: %s", p.Type())
	}
	return id.setID(p.Value().(string))
}

func (id *componentID) id() (s string) {
	s = id.name
	if !id.stream {
		s = "~" + s
	}
	return
}

func (id *componentID) setID(v string) error {
	id.name = strings.TrimPrefix(v, "~")
	id.stream = (id.name == v)
	return nil
}

// componentEntity is a hierarchial component that stores a log path
// component and is keyed on the full log stream path. See componentID
// documentation for more detailed discussion on what a component is.
//
// It is keyed on its name component with a parent whose name component is keyed
// similarly on previous components. This causes an implicit ordering by name
// such that hierarchy listings are alphabetically-ordered.
//
// Loading path components is inherently idempotent, since each one is defined
// solely by its identity. Consequently, transactions are not necessary when
// loading path components.
//
// All componentEntity share a common ancestor, "/".
//
// This entity is created at stream registration, and is entirely centered
// around being queried for log stream "directory" listings. For example:
//
//	foo/bar/+/baz/qux
//
// This stream would add three name components to the datastore, keyed
// ((parent...), id) as:
//	(("/"), "foo")
//	(("/", "foo"), "bar")
//	(("/", "foo", "bar"), "+", "baz")
//	(("/", "foo", "bar", "+", "baz"), "qux")
//
// A generated property, "s", is stored to indicate whether this entry is a
// stream or path component. This enables two types of queries:
//	1) Immediate path queries, which return all path components immediately
//	   under a hierarchy space. This can be done by ancestor query with a fixed
//	   depth equality filter.
//	2) Recursive path queries, which return all stream paths under a hierarchy
//	   root, can be done by ancestor query on the root with a "s == true"
//	   equality filter.
//
// Both of these queries can be configured via GetPathHierarchy.
type componentEntity struct {
	// _kind is the entity's kind. This is intentionally short to mitigate index
	// bloat since this will be repeated in a lot of keys.
	_kind string `gae:"$kind,_lsnc"`
	// Parent is the key of this entity's parent.
	Parent *ds.Key `gae:"$parent"`

	// Name is the name component of this specific stream.
	ID componentID `gae:"$id"`
	// D is the depth of this component in the path hierarchy.
	Depth int `gae:"D"`
	// P, if true, indicates that this log stream is purged.
	Purged bool `gae:"P"`
}

var _ ds.PropertyLoadSaver = (*componentEntity)(nil)

func (c *componentEntity) Load(pmap ds.PropertyMap) error {
	delete(pmap, "s")
	return ds.GetPLS(c).Load(pmap)
}

func (c *componentEntity) Save(withMeta bool) (ds.PropertyMap, error) {
	pmap, err := ds.GetPLS(c).Save(withMeta)
	if err != nil {
		return nil, err
	}
	pmap["s"] = []ds.Property{ds.MkProperty(c.ID.stream)}
	return pmap, nil
}

func (c *componentEntity) pathTo(depth int) string {
	size := c.Depth - depth + 1
	switch {
	case size < 0:
		return ""

	case size == 0:
		return c.ID.name
	}

	comps := make([]string, size)

	k := c.Parent
	comps[size-1] = c.ID.name
	var cid componentID
	for i := size - 2; i >= 0; i-- {
		if k == nil {
			return ""
		}

		cid.setID(k.StringID())
		comps[i] = cid.name
		k = k.Parent()
	}
	return types.Construct(comps...)
}

// streamPath returns the StreamPath of this component. It must only be called
// on a stream component.
func (c *componentEntity) streamPath() types.StreamPath {
	_, _, toks := c.Parent.Split()

	// toks starts with a common "/" ancestor, which we discard for path building.
	sidx := -1
	parts := make([]types.StreamName, len(toks))
	parts[len(parts)-1] = types.StreamName(c.ID.name)
	for i, t := range toks[1:] {
		v := strings.TrimPrefix(t.StringID, "~")
		if v == string(types.StreamPathSep) {
			sidx = i
		}
		parts[i] = types.StreamName(v)
	}
	if sidx < 0 {
		panic("no stream path separator in stream element")
	}

	return types.MakeStreamPath(parts[:sidx], parts[sidx+1:])
}

func componentTokenKey(di ds.Interface, components []string) (*ds.Key, int) {
	if len(components) == 0 {
		return di.NewKey("_lsnc", "/", 0, nil), 0
	}

	lidx := len(components) - 1
	cid := componentID{
		name: components[lidx],
	}
	parent, d := componentTokenKey(di, components[:lidx])
	return di.NewKey("_lsnc", cid.id(), 0, parent), d + 1
}

func components(p string) []string {
	if p == "" {
		return nil
	}

	prefix, sep, name := types.StreamPath(p).SplitParts()
	prefixS, nameS := prefix.Segments(), name.Segments()
	parts := make([]string, 0, len(prefixS)+len(nameS)+1)
	for _, c := range prefixS {
		parts = append(parts, string(c))
	}
	if sep {
		parts = append(parts, string(types.StreamPathSep))
		if name != "" {
			for _, c := range nameS {
				parts = append(parts, string(c))
			}
		}
	}
	return parts
}

// Put creates component entries for path.
//
// This method does not retry on datastore failure. If that behavior is desired,
// this should be run in a transaction or retry loop.
func Put(di ds.Interface, ls *coordinator.LogStream) error {
	p := ls.Path()
	if err := p.Validate(); err != nil {
		return err
	}

	// Build all path componentEntity objects for our parts.
	c := components(string(p))
	components := make([]*componentEntity, 0, len(c))
	prev, _ := componentTokenKey(di, nil)

	for i, comp := range c {
		cur := componentEntity{
			Parent: prev,
			ID: componentID{
				name: comp,
			},
			Depth: i + 1,
		}
		if i == len(c)-1 {
			// This is the stream component.
			cur.ID.stream = true
		} else {
			prev = di.NewKey("_lsnc", cur.ID.id(), 0, prev)
		}

		components = append(components, &cur)
	}
	return di.PutMulti(components)
}

// Purge sets the named path component's purged status to v.
//
// Note that a subsequent Put to the same stream will remove the purged status.
// This should not happen in practice, though, since Put should only happen on
// stream registration, which will only occur once per stream.
//
// This method does not retry on datastore failure. If that behavior is desired,
// this should be run in a transaction or retry loop.
func Purge(di ds.Interface, p types.StreamPath, v bool) error {
	if err := p.Validate(); err != nil {
		return err
	}

	comp := components(string(p))
	lidx := len(comp) - 1

	k, _ := componentTokenKey(di, comp[:lidx])
	c := componentEntity{
		Parent: k,
		ID: componentID{
			name:   comp[lidx],
			stream: true,
		},
	}

	if err := di.Get(&c); err != nil {
		return err
	}

	if c.Purged == v {
		return nil
	}

	c.Purged = v
	return di.Put(&c)
}

// Request is a listing request to execute.
//
// It describes a hierarchy listing request. For example, given the following
// streams:
//	foo/+/bar
//	foo/+/bar/baz
//	foo/bar/+/baz
//
// The following queries would return values ("$" denotes streams vs. paths):
//	Base="": ["foo"]
//	Base="foo": ["+", "bar"]
//	Base="foo/+": ["bar$", "bar"]
//	Base="foo/bar": ["+"]
//	Base="foo/bar/+": ["baz$"]
//
// When Recursive is true and StreamOnly is true, listing the same streams would
// return:
//	Base="": ["foo/+/bar", "foo/+/bar/baz", "foo/bar/+/baz"]
//	Base="foo": ["+/bar", "+/bar/baz", "bar/+/baz"]
//	Base="foo/+": ["bar", "bar/baz"]
//	Base="foo/bar": ["+/baz"]
//	Base="foo/bar/+": ["baz"]
//
// If Limit is >0, it will be used to constrain the results. Otherwise, all
// results will be returned.
//
// If Next is not empty, it is a datastore cursor for a continued query. If
// supplied, it must use the same parameters as the previous queries in the
// sequence.
type Request struct {
	// Base is the base path to list.
	Base string
	// Recursive, if true, returns all path components underneath Base recursively
	// instead of those immediately underneath Base.
	Recursive bool
	// StreamOnly, if true, only returns stream path components.
	StreamOnly bool
	// IncludePurged, if true, allows purged streams to be returned.
	IncludePurged bool

	// Limit, if >0, is the maximum number of results to return. If more results
	// are available, the returned List will have its Next field set to a cursor
	// that can be used to issue iterative requests.
	Limit int
	// Next, if not empty, is the start cursor for this stream.
	Next string
}

// Component is a single component element in the stream path hierarchy.
//
// This can represent a path component "path" in (path/component/+/stream)
// or a stream component ("stream").
type Component struct {
	// Name is the name of this hierarchy element.
	Name string
	// Stream, if not empty, indicates that this is a stream component and
	// contains the full stream path that it represents.
	Stream types.StreamPath
}

// List is a branch of the stream path tree.
type List struct {
	// Base is the hierarchy base.
	Base string
	// Comp is the set of elements in this hierarchy result.
	Comp []*Component

	// Next, if not empty, is the iterative query cursor.
	Next string
}

// Get performs a hierarchy query based on parameters in the supplied Request
// and returns the resulting List.
func Get(di ds.Interface, r Request) (*List, error) {
	// Clean any leading/trailing separators.
	r.Base = strings.Trim(r.Base, string(types.StreamNameSep))

	k, d := componentTokenKey(di, components(r.Base))
	d++

	q := ds.NewQuery("_lsnc").Ancestor(k)
	if !r.Recursive {
		q = q.Eq("D", d)
	}
	if !r.IncludePurged {
		q = q.Eq("P", false)
	}
	if r.StreamOnly {
		q = q.Eq("s", true)
	}
	if r.Next != "" {
		c, err := di.DecodeCursor(r.Next)
		if err != nil {
			return nil, err
		}
		q = q.Start(c)
	}

	limit := r.Limit
	if limit > 0 {
		q = q.Limit(int32(limit))
	}

	l := List{
		Base: r.Base,
	}
	err := di.Run(q, func(e *componentEntity, cb ds.CursorCB) error {
		// Skip the root component. This can be returned for recursive queries.
		if r.Recursive && e.Depth < d {
			return nil
		}

		c := Component{
			Name: e.pathTo(d),
		}
		if e.ID.stream {
			c.Stream = e.streamPath()
		}
		l.Comp = append(l.Comp, &c)

		if limit > 0 && len(l.Comp) >= limit {
			cursor, err := cb()
			if err != nil {
				return err
			}
			l.Next = cursor.String()
			return ds.Stop
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &l, nil
}
