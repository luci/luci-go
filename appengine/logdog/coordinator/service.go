// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/gcloud/gs"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
)

// Service is the base service container for LogDog handlers and endpoints. It
// is primarily usable as a means of consistently stubbing out various external
// components.
type Service struct {
	// StorageFunc is a function that generates an intermediate Storage instance
	// for use by this service. If nil, the production intermediate Storage
	// instance will be used.
	//
	// This is provided for testing purposes.
	StorageFunc func(context.Context) (storage.Storage, error)

	// GSClientFunc is a function that generates a Google Storage client instance
	// for use by this service. If nil, the production Google Storage Client will
	// be used.
	GSClientFunc func(context.Context) (gs.Client, error)
}

// Storage retrieves the configured Storage instance.
func (s *Service) Storage(c context.Context) (storage.Storage, error) {
	sf := s.StorageFunc
	if sf == nil {
		// Production: use BigTable storage.
		sf = config.GetStorage
	}

	st, err := sf(c)
	if err != nil {
		log.Errorf(log.SetError(c, err), "Failed to get Storage instance.")
		return nil, err
	}
	return st, nil
}

// GSClient instantiates a Google Storage client.
func (s *Service) GSClient(c context.Context) (gs.Client, error) {
	f := s.GSClientFunc
	if f == nil {
		f = gs.NewProdClient
	}

	gsc, err := f(c)
	if err != nil {
		log.Errorf(log.SetError(c, err), "Failed to get Google Storage client.")
		return nil, err
	}
	return gsc, nil
}
