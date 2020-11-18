// Copyright 2020 The LUCI Authors.
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

package gerritfake

import (
	"fmt"
	"sync"

	"google.golang.org/grpc/status"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// Fake simulates Gerrit for CV tests.
type Fake struct {
	m  sync.Mutex
	cs map[string]*Change
}

// Change = change details + ACLs.
type Change struct {
	Host string
	Info *gerritpb.ChangeInfo
	ALCs AccessCheck
}

type AccessCheck func(op Operation, luciProject string) *status.Status

type Operation int

const (
	OpRead Operation = iota
	OpComment
	OpReview
	OpSubmit
)

///////////////////////////////////////////////////////////////////////////////
// Antiboilerplate functions to reduce verbosity in tests.

// WithCLs returns Fake with several changes.
func WithCLs(cs ...*Change) *Fake {
	f := &Fake{
		cs: make(map[string]*Change, len(cs)),
	}
	for _, c := range cs {
		f.cs[c.key()] = c
	}
	return f
}

// With CIs returns Fake with a change per passed ChangeInfo sharing the same
// host and acls.
func WithCIs(host string, acl AccessCheck, ci ...*gerritpb.ChangeInfo) *Fake {
	panic("TODO(tandrii): implement")
}

// AddFrom adds all changes from another fake to the this fake and returns this
// fake.
//
// Changes are added by reference. Primarily useful to construct Fake with CLs
// on several hosts, e.g.:
//   fake := WithCIs(hostA, aclA, ciA1, ciA2).AddFrom(hostB, aclB, ciB1)
func (f *Fake) AddFrom(other *Fake) *Fake {
	panic("TODO(tandrii): implement")
}

// CI creates a new ChangeInfo with 1 patchset with status NEW and without any
// votes.
func CI(change int, mods ...CIModifier) *gerritpb.ChangeInfo {
	panic("TODO(tandrii): implement")
}

///////////////////////////////////////////////////////////////////////////////
// CI Modifiers

type CIModifier interface {
	Apply(ci *gerritpb.ChangeInfo)
}

// TODO(tandrii): implement these and more if needed.
//  * Project(p)
//  * Ref(ref)
//  * Patchset(p) -- creates P patchsets. Populates only the latest revision.
//  * AllRevisions -- populates all revisions(aka patchsets).
//  * Status(s)-- with given status
//  * UpdateTime(t) -- with givne UpdateTime.

///////////////////////////////////////////////////////////////////////////////
// Getters / Mutators

// Has returns if given change exists.
func (f *Fake) Has(host string, change int) bool {
	f.m.Lock()
	defer f.m.Unlock()
	_, ok := f.cs[key(host, change)]
	return ok
}

// Change returns a mutable Change that must exist. Panics otherwise.
//
// The returned Change can be modified, but such modification won't be atomic
// from perspective of concurrent RPCs. Recommended for use in between the RPCs.
func (f *Fake) GetChange(host string, change int) *Change {
	f.m.Lock()
	defer f.m.Unlock()
	c, ok := f.cs[key(host, change)]
	if !ok {
		panic(fmt.Errorf("CL %s/%d not found", host, change))
	}
	return c
}

// MutateChange modifies a change while holding a lock blocking concurrent RPCs.
// Change must exist. Panics otherwise.
func (f *Fake) MutateChange(host string, change int, mut func(c *Change)) {
	f.m.Lock()
	defer f.m.Unlock()
	c, ok := f.cs[key(host, change)]
	if !ok {
		panic(fmt.Errorf("CL %s/%d not found", host, change))
	}
	mut(c)
}

// DeleteChange deletes a change that must exist. Panics otherwise.
func (f *Fake) DeleteChange(host string, change int, mut func(c *Change)) {
	k := key(host, change)
	f.m.Lock()
	defer f.m.Unlock()
	c, ok := f.cs[k]
	if !ok {
		panic(fmt.Errorf("CL %s/%d not found", host, change))
	}
	delete(f.cs, k)
	mut(c)
}

///////////////////////////////////////////////////////////////////////////////
// Helpers

func (c *Change) key() string {
	return key(c.Host, int(c.Info.GetNumber()))
}

func key(host string, change int) string {
	return fmt.Sprintf("%s/%d", host, change)
}
