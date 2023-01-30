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

// Package experiments allow servers to use experimental code paths.
//
// An experiment is essentially like a boolean command line flag: it has a name
// and it is either set or not. If it is set, the server code may do something
// differently.
//
// The server accepts a repeated CLI flag `-enable-experiment <name>` to enable
// named experiments. A typical lifecycle of an experiment is tied to
// the deployment cycle:
//  1. Implement the code path guarded by an experiment. It is disabled by
//     default. So if someone deploys this code to production, nothing bad will
//     happen.
//  2. Enable the experiment on staging by passing `-enable-experiment <name>`
//     flag. Verify it works.
//  3. When promoting the staging to canary, enable the experiment on canary as
//     well. Verify it works.
//  4. Finally, enable the experiment when promoting canary to stable. Verify
//     it works.
//  5. At this point the experimental code path is running everywhere. Make it
//     default in the code. It makes `-enable-experiment <name>` noop.
//  6. When deploying this version, remove `-enable-experiment <name>` from
//     deployment configs.
//
// The difference from command line flags:
//   - An experiment is usually short lived. If it needs to stay for long, it
//     should be converted into a proper command line flag.
//   - The server ignores enabled experiments it doesn't know about. It
//     simplifies adding and removing experiments.
//   - There's better testing support.
package experiments

import (
	"context"
	"fmt"
	"sync"

	"go.chromium.org/luci/common/data/stringset"
)

// All registered experiments.
var available struct {
	m   sync.RWMutex
	exp stringset.Set
}

// A context.Context key for a set of enabled experiments.
var ctxKey = "go.chromium.org/luci/server/experiments"

// ID identifies an experiment.
//
// The only way to get an ID is to call Register or GetByName.
type ID struct {
	name string
}

// String return the experiment name.
func (id ID) String() string {
	return id.name
}

// Valid is false for zero ID{} value.
func (id ID) Valid() bool {
	return id.name != ""
}

// Enabled returns true if this experiment is enabled.
//
// In production servers an experiment is enabled by `-enable-experiment <name>`
// CLI flag.
//
// In tests an experiment can be enabled via Enable(ctx, id).
func (id ID) Enabled(ctx context.Context) bool {
	cur, _ := ctx.Value(&ctxKey).(stringset.Set)
	return cur.Has(id.name)
}

// Register is usually called during init() to declare some experiment.
//
// Panics if such experiment is already registered. The package that registered
// the experiment can then check if it is enabled in runtime via id.Enabled().
func Register(name string) ID {
	if name == "" {
		panic("experiment name can't be empty")
	}
	available.m.Lock()
	defer available.m.Unlock()
	if available.exp == nil {
		available.exp = stringset.New(1)
	}
	if !available.exp.Add(name) {
		panic(fmt.Sprintf("experiment %q is already registered, pick a different name", name))
	}
	return ID{name}
}

// GetByName returns a registered experiment given its name.
//
// Returns ok == false if such experiment hasn't been registered.
func GetByName(name string) (id ID, ok bool) {
	available.m.RLock()
	defer available.m.RUnlock()
	if available.exp.Has(name) {
		return ID{name}, true
	}
	return ID{}, false
}

// Enable enables zero or more experiments.
//
// In other words Enable(ctx, exp) returns a context `ctx` such that
// exp.Enabled(ctx) returns true. All experiments must be registered already.
//
// This is an additive operation.
func Enable(ctx context.Context, id ...ID) context.Context {
	for _, exp := range id {
		if !exp.Valid() {
			panic("invalid experiment")
		}
	}

	// Don't modify the context if the requested experiment is already enabled.
	cur, _ := ctx.Value(&ctxKey).(stringset.Set)
	enable := make([]string, 0, len(id))
	for _, exp := range id {
		if !cur.Has(exp.name) {
			enable = append(enable, exp.name)
		}
	}
	if len(enable) == 0 {
		return ctx
	}

	// Don't mutate the existing set in the parent context, make a clone.
	if cur == nil {
		cur = stringset.NewFromSlice(enable...)
	} else {
		cur = cur.Dup()
		for _, exp := range enable {
			cur.Add(exp)
		}
	}

	return context.WithValue(ctx, &ctxKey, cur)
}
