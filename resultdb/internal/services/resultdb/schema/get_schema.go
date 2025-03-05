// Copyright 2025 The LUCI Authors.
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

// Package schema defines the luci.resultdb.v1.Schemas service. It is
// hosted within the 'resultdb' GKE service.
package schema

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/config"
	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateGetSchemaRequest validates the request to get schema.
func validateGetSchemaRequest(req *pb.GetSchemaRequest) error {
	if req.Name == "" {
		return errors.Reason("name: unspecified").Err()
	}
	if req.Name != "schemas/global" {
		return errors.Reason(`name: invalid; "schemas/global" is currently the only valid schema resource name`).Err()
	}
	return nil
}

// GetSchema implements pb.SchemasServer.
func (s *schemasServer) Get(ctx context.Context, in *pb.GetSchemaRequest) (*pb.Schema, error) {
	if err := validateGetSchemaRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}

	// Construct schemes for response.
	schemes := make(map[string]*pb.Scheme, 1+len(cfg.Schemes))
	for _, scheme := range cfg.Config.GetSchemes() {
		schemes[scheme.Id] = schemeFromConfig(scheme)
	}
	// Include the legacy scheme which the user does not need to configure.
	schemes[config.LegacyScheme.Id] = schemeFromConfig(config.LegacyScheme)

	return &pb.Schema{
		Name:    "schemas/global",
		Schemes: schemes,
	}, nil
}

func schemeFromConfig(scheme *configpb.Scheme) *pb.Scheme {
	return &pb.Scheme{
		Id:                scheme.Id,
		HumanReadableName: scheme.HumanReadableName,
		Coarse:            schemeLevelFromConfig(scheme.Coarse),
		Fine:              schemeLevelFromConfig(scheme.Fine),
		Case:              schemeLevelFromConfig(scheme.Case),
	}
}

func schemeLevelFromConfig(level *configpb.Scheme_Level) *pb.Scheme_Level {
	if level == nil {
		return nil
	}
	return &pb.Scheme_Level{
		HumanReadableName: level.HumanReadableName,
		ValidationRegexp:  level.ValidationRegexp,
	}
}
