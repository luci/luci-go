// Copyright 2023 The LUCI Authors.
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

	"google.golang.org/grpc/codes"

	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/config_service/internal/validation"
	configpb "go.chromium.org/luci/config_service/proto"
)

// validator is implemented by `validation.Validator`.
type validator interface {
	Examine(context.Context, config.Set, []validation.File) (*validation.ExamineResult, error)
	Validate(context.Context, config.Set, []validation.File) (*cfgcommonpb.ValidationResult, error)
}

// ValidateConfigs validates configs. Implements configpb.ConfigsServer.
func (c Configs) ValidateConfigs(ctx context.Context, req *configpb.ValidateConfigsRequest) (*configpb.ValidateConfigsResponse, error) {
	return nil, appstatus.Errorf(codes.Unimplemented, "ValidateConfigs hasn't been implemented yet.")
}
