// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/grpcutil"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
)

// Server is the user-facing log access and query endpoint service.
type Server struct {
	// storageFunc is a function that generates a Storage instance for use by this
	// service. If nil, the production Storage will be used.
	//
	// This is provided for testing purposes.
	storageFunc func(context.Context) (storage.Storage, error)

	// queryResultLimit is the maximum number of query results to return in a
	// single query. If zero, the default will be used.
	//
	// This is provided for testing purposes.
	queryResultLimit int
}

var _ logs.LogsServer = (*Server)(nil)

// getStorage retrieves the configured Storage instance.
//
// If an error occurs, an endpoints InternalServerError will be returned.
func (s *Server) getStorage(c context.Context) (storage.Storage, error) {
	sf := s.storageFunc
	if sf == nil {
		// Production: use BigTable storage.
		sf = config.GetStorage
	}

	st, err := sf(c)
	if err != nil {
		log.Errorf(log.SetError(c, err), "Failed to get Storage instance.")
		return nil, grpcutil.Internal
	}
	return st, nil
}
