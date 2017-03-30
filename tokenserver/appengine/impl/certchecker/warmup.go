// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package certchecker

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/warmup"
	"github.com/luci/luci-go/tokenserver/appengine/impl/certconfig"
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
