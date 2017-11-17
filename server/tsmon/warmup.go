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

package tsmon

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/warmup"
)

func init() {
	warmup.Register("server/tsmon", func(c context.Context) (err error) {
		if settings := fetchCachedSettings(c); settings.Enabled && settings.ProdXAccount != "" {
			err = canActAsProdX(c, settings.ProdXAccount)
		} else {
			logging.Infof(c, "Skipping tsmon warmup, not configured")
		}
		return
	})
}
