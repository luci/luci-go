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

package impl

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/internal"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPokeRun(t *testing.T) {
	t.Parallel()

	Convey("Event pipeline works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const testRunID run.ID = "infra/1112223334445/deadbeefdeadbeef"

		test := func(trans bool) {
			if trans {
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return run.StartRun(ctx, testRunID)
				}, nil), ShouldBeNil)
			} else {
				So(run.StartRun(ctx, testRunID), ShouldBeNil)
			}

			So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 1)
			l, err := internal.NewDSSet(ctx, string(testRunID)).List(ctx)
			So(err, ShouldBeNil)
			So(l.Items, ShouldHaveLength, 1)

			ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-run-task"))
			So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 0)
			l, err = internal.NewDSSet(ctx, string(testRunID)).List(ctx)
			So(err, ShouldBeNil)
			So(l.Items, ShouldBeNil)
		}
		Convey("Non-Transactional", func() {
			test(false)
		})
		Convey("Transactional", func() {
			test(true)
		})
	})
}
