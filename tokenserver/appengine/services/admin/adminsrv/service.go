// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package adminsrv implements Admin API.
//
// Code defined here is either invoked by an administrator or by the service
// itself (via cron jobs or task queues).
package adminsrv

import (
	"github.com/luci/luci-go/appengine/gaeauth/server/gaesigner"
	"github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/appengine/machinetoken"
	"github.com/luci/luci-go/tokenserver/appengine/services/admin/certauthorities"
)

// Server implements admin.AdminServer RPC interface.
//
// It assumes authorization has happened already.
//
// Use NewServer to make a new instance.
type Server struct {
	machinetoken.InspectMachineTokenRPC

	caServer *certauthorities.Server
}

// NewServer returns Server configured for real production usage.
func NewServer(caServer *certauthorities.Server) *Server {
	return &Server{
		InspectMachineTokenRPC: machinetoken.InspectMachineTokenRPC{
			Signer: gaesigner.Signer{},
		},
		caServer: caServer,
	}
}

// ImportConfigs makes the server read its config from luci-config right now.
//
// Note that regularly configs are read in background each 5 min. ImportConfigs
// can be used to force config reread immediately. It will block until configs
// are read.
func (s *Server) ImportConfigs(c context.Context, _ *google.Empty) (*admin.ImportedConfigs, error) {
	return s.caServer.ImportConfig(c)
}
