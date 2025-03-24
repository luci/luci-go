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
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// The resource name of the schema singleton resource.
// See https://google.aip.dev/156.
const schemaSingletonResourceName = "schema"

// validateGetSchemaRequest validates the request to get schema.
func validateGetSchemaRequest(req *pb.GetSchemaRequest) error {
	if req.Name == "" {
		return errors.Reason("name: unspecified").Err()
	}
	if req.Name != schemaSingletonResourceName {
		return errors.Reason(`name: invalid; %q is currently the only valid schema resource name`, schemaSingletonResourceName).Err()
	}
	return nil
}

// GetSchema implements pb.SchemasServer.
func (s *schemasServer) Get(ctx context.Context, in *pb.GetSchemaRequest) (*pb.Schema, error) {
	// Note: this RPC is not authenticated, it is assumed all scheme information
	// is public information.

	if err := validateGetSchemaRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}

	// Construct schemes for response.
	schemes := make(map[string]*pb.Scheme, 1+len(cfg.Schemes))
	for _, scheme := range cfg.Schemes {
		schemes[scheme.ID] = schemeFromConfig(scheme)
	}

	return &pb.Schema{
		Name:    schemaSingletonResourceName,
		Schemes: schemes,
	}, nil
}
