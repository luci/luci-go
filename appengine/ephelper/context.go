// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ephelper

import (
	"fmt"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"golang.org/x/net/context"
)

type serviceCallKeyType int

var serviceCallKey serviceCallKeyType

type serviceCall struct {
	mi *endpoints.MethodInfo
}

// ServiceBase is an embeddable base class for endpoints services.
//
// Example:
//
//   type MyService struct {
//     *ServiceBase
//   }
//
//   func (s *MyService) MyCoolEndpoint(c context.Context) error {
//     c, err = s.Use(c, myMethodInfo)
//     if err != nil {
//       return err
//     }
//     ...
//   }
type ServiceBase struct {
	// Middleware is the set of middleware context manipulators to run for this
	// service.
	//
	// If nil, the middleware stack returned by DefaultMiddleware will be used.
	Middleware []Middleware
}

// Use should be called at the beginning of a Cloud Endpoint handler to
// initialize the handler's baseline Context and authenticate the call.
func (s *ServiceBase) Use(c context.Context, mi *endpoints.MethodInfo) (context.Context, error) {
	// In case the developer forgot to set it...
	if s == nil {
		return c, fmt.Errorf("no ServiceBase is configured for: %#v", mi)
	}

	// Embed our service context.
	sc := serviceCall{
		mi: mi,
	}
	c = context.WithValue(c, serviceCallKey, &sc)

	middleware := s.Middleware
	if middleware == nil {
		middleware = []Middleware{DefaultMiddleware(nil)}
	}
	for _, mw := range middleware {
		ic, err := mw(c)
		if err != nil {
			return c, err
		}
		c = ic
	}

	return c, nil
}

// MethodInfo returns the endpoints.MethodInfo for the current service call.
func MethodInfo(c context.Context) *endpoints.MethodInfo {
	if sc, ok := c.Value(serviceCallKey).(*serviceCall); ok {
		return sc.mi
	}
	return nil
}
