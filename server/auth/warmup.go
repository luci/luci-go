// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/warmup"
)

func init() {
	warmup.Register("server/auth:DBProvider", func(c context.Context) (err error) {
		if cfg := GetConfig(c); cfg != nil && cfg.DBProvider != nil {
			_, err = cfg.DBProvider(c)
		} else {
			logging.Infof(c, "DBProvider is not configured") // this is fine
		}
		return
	})

	warmup.Register("server/auth:Signer", func(c context.Context) (err error) {
		if cfg := GetConfig(c); cfg != nil && cfg.Signer != nil {
			_, err = cfg.Signer.ServiceInfo(c)
		} else {
			logging.Infof(c, "Signer is not configured") // this is fine
		}
		return
	})
}
