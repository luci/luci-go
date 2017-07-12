// Copyright 2016 The LUCI Authors.
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

// Package treapstore is a lightweight append-only in-memory key-value store
// built on top a treap (tree + heap) implementation.
//
// treapstore is specifically focused on supporting the in-memory datastore
// implementation at "github.com/luci/gae/impl/memory".
package treapstore

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"

	"github.com/luci/gtreap"
)

// emptyTreap is an empty gtreap.Treap.
var emptyTreap = gtreap.NewTreap(nil)

// Store is a general-purpose concurrently-accessible copy-on-write store built
// on top of the github.com/steveyen/gtreap treap (tree + heap) implementation.
//
// A Store is composed of a series of named Collection instances, each of which
// can hold data elements.
//
// A Store is created by calling New, and is initially in a read/write mode.
// Derivative Stores can be created by calling a Store's Snapshot capability.
// These Stores will be read-only, meaning their collections and those
// collections' data data may only be accessed, not modified.
//
// A Store's zero value is a valid read-only empty Store, but is not terribly
// useful.
type Store struct {
	collLock  *sync.RWMutex
	colls     map[string]*Collection
	collNames []string // collNames is copy-on-write while holding collLock.
}

// New creates a new read/write Store.
func New() *Store {
	return &Store{
		collLock: &sync.RWMutex{},
	}
}

// IsReadOnly returns true if this Store is read-only.
func (s *Store) IsReadOnly() bool { return s.collLock == nil }

func (s *Store) assertNotReadOnly() {
	if s.IsReadOnly() {
		panic("store is read-only")
	}
}

// Snapshot creates a read-only copy of the Store and all of its Collection
// instances. Because a Store is copy-on-write, this is a cheap operation.
func (s *Store) Snapshot() *Store {
	if s.IsReadOnly() {
		return s
	}

	s.collLock.RLock()
	defer s.collLock.RUnlock()

	// Return a read-only Store with the new Collection set.
	snap := &Store{
		collLock:  nil,
		colls:     make(map[string]*Collection, len(s.colls)),
		collNames: s.collNames,
	}

	// Create a read-only Collection for each coll.
	for k, coll := range s.colls {
		newColl := &Collection{
			name: coll.name,
		}
		newColl.setRoot(coll.currentRoot())
		snap.colls[k] = newColl
	}
	return snap
}

// GetCollectionNames returns the names of the Collections in this Store, sorted
// alphabetically.
func (s *Store) GetCollectionNames() []string {
	if s.collLock != nil {
		s.collLock.RLock()
		defer s.collLock.RUnlock()
	}

	// Clone our collection names slice.
	if len(s.collNames) == 0 {
		return nil
	}
	return append([]string(nil), s.collNames...)
}

// GetCollection returns the Collection with the specified name. If no such
// Collection exists, GetCollection will return nil.
func (s *Store) GetCollection(name string) *Collection {
	if s.collLock != nil {
		s.collLock.RLock()
		defer s.collLock.RUnlock()
	}
	return s.colls[name]
}

// CreateCollection returns a Collection with the specified name. If the
// collection already exists, or if s is read-only, CreateCollection will panic.
func (s *Store) CreateCollection(name string, compare gtreap.Compare) *Collection {
	s.assertNotReadOnly()

	s.collLock.Lock()
	defer s.collLock.Unlock()

	// Try to get the existing Collection again now that we hold the write lock.
	if _, ok := s.colls[name]; ok {
		panic(fmt.Errorf("collection %q already exists", name))
	}

	// Create a new read/write Collection.
	coll := &Collection{
		name:     name,
		rootLock: &sync.RWMutex{},
	}
	coll.setRoot(gtreap.NewTreap(compare))

	if s.colls == nil {
		s.colls = make(map[string]*Collection)
	}
	s.colls[name] = coll
	s.collNames = s.insertCollectionName(name)
	return coll
}

// insertCollectionName returns a copy of s.collNames with name inserted
// in its appropriate sorted position.
func (s *Store) insertCollectionName(name string) []string {
	sidx := sort.SearchStrings(s.collNames, name)
	r := make([]string, 0, len(s.collNames)+1)
	return append(append(append(r, s.collNames[:sidx]...), name), s.collNames[sidx:]...)
}

// Collection is a collection of Items.
//
// Collections belonging to read/write Store instances are, themselves,
// read/write. Collections belonging to Snapshot instances are read-only.
//
// A Collection's zero value is a valid read-only empty Collection, which is not
// terribly useful.
type Collection struct {
	name string

	rootLock *sync.RWMutex
	root     *gtreap.Treap
}

func (c *Collection) assertNotReadOnly() {
	if c.IsReadOnly() {
		panic("collection is read-only")
	}
}

// Name returns this Collection's name.
func (c *Collection) Name() string { return c.name }

// IsReadOnly returns true if this Collection is read-only.
func (c *Collection) IsReadOnly() bool { return c.rootLock == nil }

// Min returns the smallest item in the collection.
func (c *Collection) Min() gtreap.Item {
	root := c.currentRoot()
	if root == nil {
		return nil
	}
	return root.Min()
}

// Max returns the largest item in the collection.
func (c *Collection) Max() gtreap.Item {
	root := c.currentRoot()
	if root == nil {
		return nil
	}
	return root.Max()
}

// Get returns the item in the Store that matches i, or nil if no such item
// exists.
func (c *Collection) Get(i gtreap.Item) gtreap.Item {
	root := c.currentRoot()
	if root == nil {
		return nil
	}
	return root.Get(i)
}

// Put adds an item to the Store.
//
// If the Store is read-only, Put will panic.
func (c *Collection) Put(i gtreap.Item) {
	c.assertNotReadOnly()

	// Lock around the entire Upsert operation to serialize Puts.
	priority := rand.Int()
	c.rootLock.Lock()
	c.root = c.root.Upsert(i, priority)
	c.rootLock.Unlock()
}

// Delete deletes an item from the Collection, if such an item exists.
func (c *Collection) Delete(i gtreap.Item) {
	c.assertNotReadOnly()

	c.rootLock.Lock()
	c.root = c.root.Delete(i)
	c.rootLock.Unlock()
}

func (c *Collection) currentRoot() *gtreap.Treap {
	if c.rootLock != nil {
		c.rootLock.RLock()
		defer c.rootLock.RUnlock()
	}
	return c.root
}

func (c *Collection) setRoot(root *gtreap.Treap) {
	if c.rootLock != nil {
		c.rootLock.Lock()
		defer c.rootLock.Unlock()
	}
	c.root = root
}

// VisitAscend traverses the Collection ascendingly, invoking visitor for each
// visited item.
//
// If visitor returns false, iteration will stop prematurely.
//
// VisitAscend is a more efficient traversal than using an Iterator, and is
// useful in times when entry-by-entry iteration is not required.
func (c *Collection) VisitAscend(pivot gtreap.Item, visitor gtreap.ItemVisitor) {
	c.currentRoot().VisitAscend(pivot, visitor)
}

// Iterator returns an iterator over the Collection, starting at the supplied
// pivot item.
func (c *Collection) Iterator(pivot gtreap.Item) *gtreap.Iterator {
	root := c.currentRoot()
	if root == nil {
		root = emptyTreap
	}
	return root.Iterator(pivot)
}
