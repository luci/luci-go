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

package metric

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSequenceMetrics(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		store := tsmon.Store(ctx)

		Convey("sequence_number/gen_duration", func() {
			SeqNumGenDuration.Add(ctx, 33, "chromium/ci/build")
			val := store.Get(ctx, SeqNumGenDuration, time.Time{}, []interface{}{"chromium/ci/build"})
			So(val.(*distribution.Distribution).Sum(), ShouldEqual, 33)
		})
	})
}
