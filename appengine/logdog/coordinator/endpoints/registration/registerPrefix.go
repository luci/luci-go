// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package registration

import (
	"github.com/luci/luci-go/common/api/logdog_coordinator/registration/v1"
	"github.com/luci/luci-go/common/grpcutil"
	"golang.org/x/net/context"
)

func (s *server) RegisterPrefix(c context.Context, req *logdog.RegisterPrefixRequest) (*logdog.RegisterPrefixResponse, error) {
	// TODO(dnj): Actually implement this endpoint.
	return nil, grpcutil.Unimplemented
}
