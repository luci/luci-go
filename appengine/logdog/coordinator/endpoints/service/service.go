// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/ephelper"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

var (
	// Scopes is the set of OAuth scopes required by the service endpoints.
	Scopes = []string{
		endpoints.EmailScope,
	}

	// Info is the service information.
	Info = endpoints.ServiceInfo{
		Version:     "v1",
		Description: "LogDog Service API",
	}

	// MethodInfoMap maps method names to their MethodInfo structures.
	MethodInfoMap = ephelper.MethodInfoMap{
		"LoadStream": &endpoints.MethodInfo{
			Name:       "LoadStream",
			Desc:       "Loads log stream metadata.",
			HTTPMethod: "GET",
			Scopes:     Scopes,
		},

		"RegisterStream": &endpoints.MethodInfo{
			Name:       "RegisterStream",
			Path:       "registerStream",
			Desc:       "Registers a log stream.",
			HTTPMethod: "PUT",
			Scopes:     Scopes,
		},

		"TerminateStream": &endpoints.MethodInfo{
			Name:       "TerminateStream",
			Path:       "terminateStream",
			Desc:       "Register a log stream's terminal index.",
			HTTPMethod: "PUT",
			Scopes:     Scopes,
		},

		"GetConfig": &endpoints.MethodInfo{
			Name:   "GetConfig",
			Path:   "getConfig",
			Desc:   "Load service configuration parameters.",
			Scopes: Scopes,
		},
	}
)

// Auth is endpoint middleware that asserts that the current user is a member of
// the configured group.
func Auth(c context.Context) error {
	if err := config.IsServiceUser(c); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to authenticate user as a backend service.")
		if !config.IsMembershipError(err) {
			// Not a membership error. Something went wrong on the server's end.
			return endpoints.InternalServerError
		}
		return endpoints.ForbiddenError
	}
	return nil
}

// Service is a Cloud Endpoint service supporting the "service" endpoint.
//
// This endpoint is restricted to LogDog backend services.
type Service struct {
	ephelper.ServiceBase
}
