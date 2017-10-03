// Copyright 2017 The LUCI Authors.
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

package engine

import (
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTriageOp(t *testing.T) {
	t.Parallel()

	Convey("with fake env", t, func() {
		c := newTestContext(epoch)

		Convey("noop triage", func() {
			before := &Job{
				JobID:             "job",
				ActiveInvocations: []int64{1, 2, 3},
			}
			after, err := runTestTriage(c, before)
			So(err, ShouldBeNil)
			So(after, ShouldResemble, before)
		})

		Convey("pops finished invocations", func() {
			before := &Job{
				JobID:             "job",
				ActiveInvocations: []int64{1, 2, 3, 4, 5},
			}
			recentlyFinishedSet(c, before.JobID).Add(c, []int64{1, 2, 4})

			after, err := runTestTriage(c, before)
			So(err, ShouldBeNil)
			So(after.ActiveInvocations, ShouldResemble, []int64{3, 5})
		})
	})
}

func runTestTriage(c context.Context, before *Job) (after *Job, err error) {
	if err := datastore.Put(c, before); err != nil {
		return nil, err
	}

	op := triageOp{jobID: before.JobID}
	if err := op.prepare(c); err != nil {
		return nil, err
	}

	err = runTxn(c, func(c context.Context) error {
		job := Job{JobID: op.jobID}
		if err := datastore.Get(c, &job); err != nil {
			return err
		}
		if err := op.transaction(c, &job); err != nil {
			return err
		}
		return datastore.Put(c, &job)
	})
	if err != nil {
		return nil, err
	}

	op.finalize(c)

	after = &Job{JobID: before.JobID}
	if err := datastore.Get(c, after); err != nil {
		return nil, err
	}
	return after, nil
}
