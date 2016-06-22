// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package config

import (
	"net/http"

	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/router"
)

// WithConfig is a middleware that installs the LogDog Coordinator
// configuration into the Context.
func WithConfig(c *router.Context, next router.Handler) {
	if err := UseConfig(&c.Context); err != nil {
		log.WithError(err).Errorf(c.Context, "Failed to install service configuration.")
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	next(c)
}
