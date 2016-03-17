// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/api/tokenserver/v1"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/logging"
	google_protobuf "github.com/luci/luci-go/common/proto/google"
)

// Server implements tokenserver.AdminServer RPC interface.
//
// It assumes authorization has happened already. Use DecoratedAdmin to plug it
// in.
type Server struct {
	// ConfigFactory returns instances of config.Interface on demand.
	ConfigFactory func(context.Context) (config.Interface, error)
}

// ReadConfig makes the server to read its config from luci-config right now.
//
// Regularly configs are read via 5 min cron. Note that ReadConfig will block
// until configs are read.
func (a *Server) ReadConfig(c context.Context, _ *google_protobuf.Empty) (*tokenserver.ReadConfigResponse, error) {
	logging.Infof(c, "Reading configs...")
	// TODO(vadimsh): Implement.
	return &tokenserver.ReadConfigResponse{}, nil
}
