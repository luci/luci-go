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

package dm

// TimestampPurger is for testing: invoking this on a struct in this package
// will remove all timestamps from it. This is useful for testing where the
// timestamps are frequently just noise.
type TimestampPurger interface {
	PurgeTimestamps()
}

// PurgeTimestamps implements TimestampPurger.
func (g *GraphData) PurgeTimestamps() {
	if g == nil {
		return
	}
	for _, q := range g.Quests {
		q.PurgeTimestamps()
	}
}

// PurgeTimestamps implements TimestampPurger.
func (q *Quest) PurgeTimestamps() {
	if q == nil {
		return
	}
	q.Data.PurgeTimestamps()
	for _, a := range q.Attempts {
		a.PurgeTimestamps()
	}
}

// PurgeTimestamps implements TimestampPurger.
func (qd *Quest_Data) PurgeTimestamps() {
	if qd == nil {
		return
	}
	qd.Created = nil
}

// PurgeTimestamps implements TimestampPurger.
func (a *Attempt) PurgeTimestamps() {
	if a == nil {
		return
	}
	a.Data.PurgeTimestamps()

	for _, e := range a.Executions {
		e.PurgeTimestamps()
	}
}

// PurgeTimestamps implements TimestampPurger.
func (ad *Attempt_Data) PurgeTimestamps() {
	if ad == nil {
		return
	}
	ad.Created = nil
	ad.Modified = nil
	if p, _ := ad.AttemptType.(TimestampPurger); p != nil {
		p.PurgeTimestamps()
	}
}

// PurgeTimestamps implements TimestampPurger.
func (f *Attempt_Data_Finished_) PurgeTimestamps() {
	if f.Finished.GetData() == nil {
		return
	}
	f.Finished.Data.Expiration = nil
}

// PurgeTimestamps implements TimestampPurger.
func (e *Execution) PurgeTimestamps() {
	if e == nil {
		return
	}
	e.Data.PurgeTimestamps()
}

// PurgeTimestamps implements TimestampPurger.
func (ed *Execution_Data) PurgeTimestamps() {
	if ed == nil {
		return
	}
	ed.Created = nil
	ed.Modified = nil
}
