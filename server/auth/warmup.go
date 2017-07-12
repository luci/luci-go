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
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/warmup"
)

func init() {
	warmup.Register("server/auth:DBProvider", func(c context.Context) (err error) {
		if cfg := getConfig(c); cfg != nil && cfg.DBProvider != nil {
			_, err = cfg.DBProvider(c)
		} else {
			logging.Infof(c, "DBProvider is not configured") // this is fine
		}
		return
	})

	warmup.Register("server/auth:Signer", func(c context.Context) (err error) {
		if cfg := getConfig(c); cfg != nil && cfg.Signer != nil {
			_, err = cfg.Signer.ServiceInfo(c)
		} else {
			logging.Infof(c, "Signer is not configured") // this is fine
		}
		return
	})
}
