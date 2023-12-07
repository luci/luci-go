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

package auth

import (
	"context"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/warmup"
)

func init() {
	warmup.Register("server/auth:DBProvider", func(ctx context.Context) (err error) {
		if cfg := getConfig(ctx); cfg != nil && cfg.DBProvider != nil {
			_, err = cfg.DBProvider(ctx)
		} else {
			logging.Infof(ctx, "DBProvider is not configured") // this is fine
		}
		return
	})

	warmup.Register("server/auth:Signer", func(ctx context.Context) (err error) {
		if cfg := getConfig(ctx); cfg != nil && cfg.Signer != nil {
			_, err = cfg.Signer.ServiceInfo(ctx)
		} else {
			logging.Infof(ctx, "Signer is not configured") // this is fine
		}
		return
	})
}
