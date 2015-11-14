// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

// Data is the generic json data return type from most DM json endpoints.
type Data struct {
	Distributors   DistributorSlice
	Quests         QuestSlice
	Attempts       AttemptSlice
	AttemptResults AttemptResultSlice
	Executions     ExecutionsForAttemptSlice
	FwdDeps        DepsFromAttemptSlice
	BackDeps       DepsFromAttemptSlice

	Timeout   bool
	HadErrors bool

	More bool `endpoints:"desc=Indicates that the query has more results."`
}

// Merge merges `o` into `d`, field-by-field, and returns a new Data object
// containing the set of entries which were actually merged (i.e. the diff). If
// nothing new was added to `d`, this returns nil.
func (d *Data) Merge(o *Data) *Data {
	didSomething := false
	ret := &Data{}

	for _, dt := range o.Distributors {
		if ret.Distributors.Merge(d.Distributors.Merge(dt)) != nil {
			didSomething = true
		}
	}
	for _, q := range o.Quests {
		if ret.Quests.Merge(d.Quests.Merge(q)) != nil {
			didSomething = true
		}
	}
	for _, a := range o.Attempts {
		if ret.Attempts.Merge(d.Attempts.Merge(a)) != nil {
			didSomething = true
		}
	}
	for _, a := range o.AttemptResults {
		if ret.AttemptResults.Merge(d.AttemptResults.Merge(a)) != nil {
			didSomething = true
		}
	}
	for _, e := range o.Executions {
		if ret.Executions.Merge(d.Executions.Merge(e)) != nil {
			didSomething = true
		}
	}
	for _, fd := range o.FwdDeps {
		if ret.FwdDeps.Merge(d.FwdDeps.Merge(fd)) != nil {
			didSomething = true
		}
	}
	for _, bd := range o.BackDeps {
		if ret.BackDeps.Merge(d.BackDeps.Merge(bd)) != nil {
			didSomething = true
		}
	}

	if didSomething {
		return ret
	}
	return nil
}
