// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ephelper

import (
	"fmt"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/server/auth"
	"golang.org/x/net/context"
)

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
	// Authenticator, if not nil, is the Authenticator instance to use for this
	// service.
	//
	// If nil, no Authenticator will be installed into the Context.
	Authenticator *auth.Authenticator

	// InstallServices, if not nil, is the setup function called in Use to install
	// a baseline set of services into the Context. This can be used for testing
	// to install testing services into the context.
	//
	// If nil, gaemiddleware.WithProd will be used to install production services.
	InstallServices func(c context.Context) (context.Context, error)
}

// Use should be called at the beginning of a Cloud Endpoint handler to
// initialize the handler's baseline Context.
func (s *ServiceBase) Use(c context.Context, mi *endpoints.MethodInfo) (context.Context, error) {
	// In case the developer forgot to set it...
	if s == nil {
		return c, fmt.Errorf("no ServiceBase is configured for: %#v", mi)
	}

	req := endpoints.HTTPRequest(c)
	if mi != nil {
		c = auth.WithOAuthScopes(c, mi.Scopes...)
	}

	// If an initializer is configured, use it.
	if s.InstallServices != nil {
		err := error(nil)
		c, err = s.InstallServices(c)
		if err != nil {
			return c, err
		}
	} else {
		// Otherwise, use Prod.
		c = gaemiddleware.WithProd(c, req)
	}

	// Install and authenticate.
	if s.Authenticator != nil {
		c = auth.SetAuthenticator(c, s.Authenticator)

		err := error(nil)
		c, err = s.Authenticator.Authenticate(c, req)
		if err != nil {
			return c, err
		}
	}

	return c, nil
}
