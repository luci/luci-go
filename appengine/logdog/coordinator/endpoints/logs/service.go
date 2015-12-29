// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/ephelper"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
)

var (
	// Scopes is the set of OAuth scopes required by the service endpoints.
	Scopes = []string{
		endpoints.EmailScope,
	}

	// Info is the endpoint service information for the logs API.
	Info = endpoints.ServiceInfo{
		Version:     "v1",
		Description: "LogDog Log Stream API",
	}

	// MethodInfoMap is the ephelper.MethodInfoMap for this endpoint.
	MethodInfoMap = ephelper.MethodInfoMap{
		"Get": &endpoints.MethodInfo{
			Desc:       "Get log stream data.",
			HTTPMethod: "GET",
			Scopes:     Scopes,
		},

		"Query": &endpoints.MethodInfo{
			Desc:   "Query for log streams.",
			Scopes: Scopes,
		},
	}
)

// Logs is the user-facing log access and query endpoint service.
type Logs struct {
	ephelper.ServiceBase

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

// getStorage retrieves the configured Storage instance.
//
// If an error occurs, an endpoints InternalServerError will be returned.
func (s *Logs) getStorage(c context.Context) (storage.Storage, error) {
	sf := s.storageFunc
	if sf == nil {
		// Production: use BigTable storage.
		sf = config.GetStorage
	}

	st, err := sf(c)
	if err != nil {
		log.Errorf(log.SetError(c, err), "Failed to get Storage instance.")
		return nil, endpoints.InternalServerError
	}
	return st, nil
}
