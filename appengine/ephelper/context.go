// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ephelper

import (
	"fmt"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	gaeauth "github.com/luci/luci-go/appengine/gaeauth/server"
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
	// InstallServices, if not nil, is the setup function called in Use to install
	// a baseline set of services into the Context. This can be used for testing
	// to install testing services into the context.
	//
	// If nil, gaemiddleware.WithProd will be used to install production services.
	InstallServices func(c context.Context) (context.Context, error)

	// Authenticator, if not nil, is the auth.Authenticator factory to use for
	// this service. For each incoming request it returns a list of authentication
	// methods to try when authenticating that request.
	//
	// If nil, default OAuth2 authentication will be used.
	Authenticator func(c context.Context, mi *endpoints.MethodInfo) auth.Authenticator
}

// Use should be called at the beginning of a Cloud Endpoint handler to
// initialize the handler's baseline Context and authenticate the call.
func (s *ServiceBase) Use(c context.Context, mi *endpoints.MethodInfo) (context.Context, error) {
	// In case the developer forgot to set it...
	if s == nil {
		return c, fmt.Errorf("no ServiceBase is configured for: %#v", mi)
	}

	req := endpoints.HTTPRequest(c)

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
	authenticator := (auth.Authenticator)(nil)
	if s.Authenticator == nil {
		authenticator = auth.Authenticator{
			&gaeauth.OAuth2Method{Scopes: mi.Scopes},
		}
	} else {
		authenticator = s.Authenticator(c, mi)
	}
	c = auth.SetAuthenticator(c, authenticator)
	return authenticator.Authenticate(c, req)
}
