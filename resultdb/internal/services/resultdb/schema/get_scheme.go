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
	"regexp"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var schemeNameRe = regexp.MustCompile("^schema/schemes/(" + config.SchemeIDPattern + ")$")

// validateGetSchemaRequest validates the request to get schema.
func validateGetSchemeRequest(req *pb.GetSchemeRequest) error {
	_, err := parseSchemeName(req.Name)
	if err != nil {
		return errors.Annotate(err, "name").Err()
	}
	return nil
}

// GetScheme implements pb.SchemasServer.
func (s *schemasServer) GetScheme(ctx context.Context, in *pb.GetSchemeRequest) (*pb.Scheme, error) {
	// Note: this RPC is not authenticated, it is assumed all scheme information
	// is public information.

	if err := validateGetSchemeRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	schemeID, err := parseSchemeName(in.Name)
	if err != nil {
		// Should never happen, request already validated.
		return nil, err
	}

	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}

	scheme, ok := cfg.Schemes[schemeID]
	if !ok {
		return nil, appstatus.Errorf(codes.NotFound, "scheme with ID %q not found", schemeID)
	}
	return scheme.ToProto(), nil
}

// parseSchemeName parses a scheme resource name into its constituent ID.
func parseSchemeName(name string) (schemeID string, err error) {
	if name == "" {
		return "", errors.Reason("unspecified").Err()
	}
	match := schemeNameRe.FindStringSubmatch(name)
	if match == nil {
		return "", errors.Reason("invalid scheme name, expected format: %q", schemeNameRe).Err()
	}
	return match[1], nil
}
