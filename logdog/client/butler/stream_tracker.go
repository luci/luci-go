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

package butler

import (
	"sort"
	"strings"
	"sync"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/logdog/common/types"
)

var errClosedNamespace = errors.New("already closed")
var errAlreadyRegistered = errors.New("duplicate registration")

type namespaceTracker struct {
	closer   chan struct{}
	draining stringset.Set
}

type streamTracker struct {
	mu sync.RWMutex

	seen     stringset.Set
	running  stringset.Set
	draining stringset.Set

	closedNamespaces map[types.StreamName]*namespaceTracker
}

func newStreamTracker() *streamTracker {
	ret := &streamTracker{
		seen:     stringset.New(100),
		running:  stringset.New(100),
		draining: stringset.New(100),

		closedNamespaces: map[types.StreamName]*namespaceTracker{},
	}
	return ret
}

func (s *streamTracker) CanRegister(name types.StreamName) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.canRegisterLocked(name)
}

func (s *streamTracker) canRegisterLocked(name types.StreamName) error {
	for _, ns := range name.Namespaces() {
		if _, closed := s.closedNamespaces[ns]; closed {
			return errors.Fmt("namespace %q: %w", ns, errClosedNamespace)
		}
	}
	return nil
}

func (s *streamTracker) RegisterStream(name types.StreamName) error {
	if err := s.CanRegister(name); err != nil {
		return errors.Fmt("stream %q: %w", name, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.canRegisterLocked(name); err != nil {
		return errors.Fmt("stream %q: %w", name, err)
	}

	if s.seen.Has(string(name)) {
		return errors.Fmt("stream %q: %w", name, errAlreadyRegistered)
	}

	s.seen.Add(string(name))
	s.running.Add(string(name))
	s.draining.Add(string(name))
	return nil
}

func (s *streamTracker) CompleteStream(name types.StreamName) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nameS := string(name)

	s.running.Del(nameS)
}

func (s *streamTracker) DrainedStream(name types.StreamName) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nameS := string(name)

	s.running.Del(nameS)
	s.draining.Del(nameS)

	for _, ns := range name.Namespaces() {
		if tracker, ok := s.closedNamespaces[ns]; ok {
			if tracker.draining.Del(nameS) {
				if tracker.draining.Len() == 0 {
					close(tracker.closer)
				}
			}
		}
	}
}

func (s *streamTracker) DrainNamespace(ns types.StreamName) <-chan struct{} {
	ns = ns.AsNamespace()

	s.mu.RLock()
	cur := s.closedNamespaces[ns]
	s.mu.RUnlock()
	if cur != nil {
		return cur.closer
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if cur := s.closedNamespaces[ns]; cur != nil {
		return cur.closer
	}

	tracker := &namespaceTracker{
		draining: stringset.New(s.draining.Len()),
		closer:   make(chan struct{}),
	}
	s.closedNamespaces[ns] = tracker

	s.draining.Iter(func(name string) bool {
		if strings.HasPrefix(name, string(ns)) {
			tracker.draining.Add(name)
		}
		return true
	})

	if tracker.draining.Len() == 0 {
		close(tracker.closer)
	}
	return tracker.closer
}

// Seen returns a sorted list of stream names that have been registered.
func (s *streamTracker) Seen() []types.StreamName {
	var streams types.StreamNameSlice
	func() {
		s.mu.RLock()
		defer s.mu.RUnlock()
		streams = make([]types.StreamName, 0, s.seen.Len())

		s.seen.Iter(func(s string) bool {
			streams = append(streams, types.StreamName(s))
			return true
		})
	}()

	sort.Sort(streams)
	return ([]types.StreamName)(streams)
}

func (s *streamTracker) RunningCount() int {
	// TODO(iannucci): the use of this function is racy; the Butler should have
	// a very distinct shutdown sequence of:
	//   * Close() - no new streams are allowed to be registered.
	//   * WaitDrained() - all streams are done draining.
	//
	// Currently the shutdown process "activates" the current runStreams, meaning
	// it will terminate when there are no running streams, THEN it shuts down the
	// stream server (preventing new registrations), THEN it runs another
	// runStreams to "clean up" any remaining streams.
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running.Len()
}

// Draining returns a sorted list of stream names that are still draining in the
// given namespace.
func (s *streamTracker) Draining(ns types.StreamName) []types.StreamName {
	ns = ns.AsNamespace()
	nsS := string(ns)

	var streams types.StreamNameSlice
	func() {
		s.mu.RLock()
		defer s.mu.RUnlock()
		streams = make([]types.StreamName, 0, s.seen.Len())

		s.draining.Iter(func(s string) bool {
			if strings.HasPrefix(s, nsS) {
				streams = append(streams, types.StreamName(s))
			}
			return true
		})
	}()

	sort.Sort(streams)
	return ([]types.StreamName)(streams)
}
