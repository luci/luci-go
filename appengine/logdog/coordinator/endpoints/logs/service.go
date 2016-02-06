// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
)

// Server is the user-facing log access and query endpoint service.
type Server struct {
	coordinator.Service

	// queryResultLimit is the maximum number of query results to return in a
	// single query. If zero, the default will be used.
	//
	// This is provided for testing purposes.
	queryResultLimit int
}

var _ logs.LogsServer = (*Server)(nil)
