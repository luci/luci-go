// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ephelper

import (
	"errors"
	"fmt"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
)

const errPrefix = "endpoints.Register: "

var (
	// ErrServerNil is returned if you pass a nil server to Register. Don't do
	// that.
	ErrServerNil = errors.New(errPrefix + "server is nil")

	// ErrServiceNil is returned if you pass a nil service to Register. Don't do
	// that.
	ErrServiceNil = errors.New(errPrefix + "service is nil")
)

// MethodInfoMap is the common registry for an endpoints service. It's
// used by infra/libs/endpoints_client to populate its API.
type MethodInfoMap map[string]*endpoints.MethodInfo

// Register adds an endpoints.RegisterService-compatible service object using
// the MethodInfoMap to look up the MethodInfo objects by methodName. It is
// intended to be called at init()-time of an app or client which relies on
// these endpoints.
//
// service should be an instance of your service type (as if you were passing
// it to "go-endpoints/endpoints".RegisterService).
func Register(server *endpoints.Server, service interface{}, si *endpoints.ServiceInfo, mi MethodInfoMap) error {
	if server == nil {
		return ErrServerNil
	}
	if service == nil {
		return ErrServiceNil
	}
	if si == nil {
		si = &endpoints.ServiceInfo{Default: true}
	}

	api, err := server.RegisterService(service, si.Name, si.Version,
		si.Description, si.Default)
	if err != nil {
		return err
	}

	for methodName, info := range mi {
		method := api.MethodByName(methodName)
		if method == nil {
			return fmt.Errorf(
				errPrefix+"no method %q (did you forget to export it?)", methodName)
		}
		curInfo := method.Info()
		// These three are set automatically based on reflection, so only override
		// them if the info object contains something new.
		if info.Name == "" {
			info.Name = curInfo.Name
		}
		if info.Path == "" {
			info.Path = curInfo.Path
		}
		if info.HTTPMethod == "" {
			info.HTTPMethod = curInfo.HTTPMethod
		}
		*curInfo = *info
		mi[methodName] = curInfo // So that we can observe the merged result
	}

	return nil
}
