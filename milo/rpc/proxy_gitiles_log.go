// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpc

import (
	"context"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/server/auth"

	milopb "go.chromium.org/luci/milo/proto/v1"
)

// ProxyGitilesLog implements milopb.MiloInternal service
func (s *MiloInternalService) ProxyGitilesLog(ctx context.Context, req *milopb.ProxyGitilesLogRequest) (_ *gitiles.LogResponse, err error) {
	if err := validateProxyGitilesLogRequest(req); err != nil {
		return nil, err
	}

	// The ACL check, request validation happens on the gitiles server.
	gitilesClient, err := s.GetGitilesClient(ctx, req.Host, auth.AsCredentialsForwarder)
	if err != nil {
		return nil, err
	}
	return gitilesClient.Log(ctx, req.Request)
}

func validateProxyGitilesLogRequest(req *milopb.ProxyGitilesLogRequest) error {
	if req.GetHost() == "" {
		return errors.New("host is required")
	}
	if !strings.HasSuffix(req.Host, ".googlesource.com") {
		return errors.New("host must be a subdomain of .googlesource.com")
	}

	// We only validate that the request is defined.
	//
	// Field validation is delegated to the actual gitiles service implementation.
	if req.Request == nil {
		return errors.New("request is required")
	}

	return nil
}
