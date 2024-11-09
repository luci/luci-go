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

package certchecker

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/warmup"

	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"
)

func init() {
	warmup.Register("tokenserver/appengine/impl/certchecker", func(c context.Context) error {
		names, err := certconfig.ListCAs(c)
		if err != nil {
			return err
		}
		var merr errors.MultiError
		for _, cn := range names {
			logging.Infof(c, "Warming up %q", cn)
			checker, err := GetCertChecker(c, cn)
			if err == nil {
				_, err = checker.GetCA(c)
			}
			if err != nil {
				logging.WithError(err).Warningf(c, "Failed to warm up %q", cn)
				merr = append(merr, err)
			}
		}
		if len(merr) == 0 {
			return nil
		}
		return merr
	})
}
