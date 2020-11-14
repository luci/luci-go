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

package git

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGraphCache(t *testing.T) {
	t.Parallel()

	Convey(`GraphCache`, t, func() {
		ctx := context.Background()
		ctx = memlogger.Use(ctx)

		Convey(`empty file is cache-miss`, func() {
			tmpd, err := ioutil.TempDir("", "filegraph_git")
			So(err, ShouldBeNil)
			defer os.RemoveAll(tmpd)

			var cache graphCache
			cache.File, err = os.Create(filepath.Join(tmpd, "empty"))
			So(err, ShouldBeNil)
			defer cache.Close()

			_, err = cache.tryReading(ctx)
			So(err, ShouldBeNil)

			log := logging.Get(ctx).(*memlogger.MemLogger)
			So(log, memlogger.ShouldHaveLog, logging.Info, "populating cache")
		})
	})
}
