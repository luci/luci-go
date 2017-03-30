// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/warmup"
)

func init() {
	warmup.Register("tokenserver/appengine/impl/delegation", func(c context.Context) error {
		_, err := DelegationConfigLoader(c)
		return err
	})
}
