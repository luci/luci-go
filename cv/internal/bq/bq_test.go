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
	"testing"
	"time"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBQ(t *testing.T) {
	t.Parallel()

	// The test will include:
	// - Creation of a real inserter without sending any real rows
	// - Usage of the mock inserter

	// And then:
	// Same thing but fail in each different way and assert
	// that an error was returned and nothing was sent.
	Convey("NewInserter", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		// Expected usage is that a new Inserter would be created in the app's main function and used for all BQ row sending.

		NewInserter
	})

	Convey("Mock Inserter", t, func() {
		// The mock inserter can be used in tests; it records which
		// rows have been sent without actually
	})
	//
}
