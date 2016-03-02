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

	// resultLimit is the maximum number of query results to return in a
	// single query. If zero, the default will be used.
	//
	// This is provided for testing purposes.
	resultLimit int
}

func (s *Server) limit(v int, d int) int {
	if s.resultLimit > 0 {
		d = s.resultLimit
	}
	if v <= 0 || v > d {
		return d
	}
	return v
}

var _ logdog.LogsServer = (*Server)(nil)
