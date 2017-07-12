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

package cfgclient

import (
	"testing"

	"github.com/luci/gae/impl/memory"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestServiceConfigSet(t *testing.T) {
	t.Parallel()

	Convey(`With a testing AppEngine context`, t, func() {
		c := memory.Use(context.Background())

		Convey(`Can get the current service config set.`, func() {
			So(CurrentServiceConfigSet(c), ShouldEqual, "services/app")
		})
	})
}
