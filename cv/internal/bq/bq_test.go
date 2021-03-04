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

package bq

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	//"go.chromium.org/luci/common/data/stringset"

	. "github.com/smartystreets/goconvey/convey"
)

func testingContext() context.Context {
	c := txndefer.FilterRDS(memory.Use(context.Background()))
	datastore.GetTestable(c).AutoIndex(true)
	datastore.GetTestable(c).Consistent(true)
	c = clock.Set(c, testclock.New(time.Unix(1442270520, 0).UTC()))
	c = mathrand.Set(c, rand.New(rand.NewSource(1000)))
	return c
}

func TestSet(t *testing.T) {
	t.Parallel()

	Convey("item one lifecycle", t, func() {
		//c := testingContext()
	})
}
