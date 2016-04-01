// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/server/middleware"
)

const (
	// defaultMultiTaskBatchSize is the default value for Backend's
	// multiTaskBatchSize parameter.
	defaultMultiTaskBatchSize = 100
)

// Backend is the base struct for all Backend handlers. It is mostly used to
// configure testing parameters.
type Backend struct {
	// multiTaskBatchSize is the number of batch tasks to create at a time.
	multiTaskBatchSize int

	// s is the backing Coordinator service base.
	s coordinator.Service
}

func (b *Backend) getMultiTaskBatchSize() int {
	if v := b.multiTaskBatchSize; v > 0 {
		return v
	}
	return defaultMultiTaskBatchSize
}

// InstallHandlers installs handlers for the Backend.
func (b *Backend) InstallHandlers(r *httprouter.Router, h middleware.Base) {
	r.GET("/archive/cron/terminal", h(gaemiddleware.RequireCron(b.HandleArchiveCron)))
	r.GET("/archive/cron/nonterminal", h(gaemiddleware.RequireCron(b.HandleArchiveCronNT)))
	r.GET("/archive/cron/purge", h(b.HandleArchiveCronPurge))
}
