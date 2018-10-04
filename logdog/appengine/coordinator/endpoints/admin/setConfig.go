// Copyright 2015 The LUCI Authors.
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

package admin

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/admin/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator/config"

	"google.golang.org/grpc/codes"
)

// SetConfig loads the supplied configuration into a config.GlobalConfig
// instance.
func (s *server) SetConfig(c context.Context, req *logdog.SetConfigRequest) (*empty.Empty, error) {
	se := config.Settings{
		BigTableServiceAccountJSON: req.StorageServiceAccountJson,
	}
	if err := se.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "New configuration did not validate.")
		return nil, grpcutil.Errf(codes.InvalidArgument, "config did not validate: %v", err)
	}

	if err := se.Store(c, "setConfig endpoint"); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to store new configuration.")
		return nil, grpcutil.Internal
	}
	return &empty.Empty{}, nil
}
