// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"github.com/luci/luci-go/common/api/logdog_coordinator/admin/v1"
)

// Server is the Cloud Endpoint service structure for the administrator endpoint.
type Server struct{}

var _ admin.AdminServer = (*Server)(nil)
