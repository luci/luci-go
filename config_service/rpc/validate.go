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
	"crypto/sha256"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config_service/internal/acl"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/internal/validation"
	configpb "go.chromium.org/luci/config_service/proto"
)

// validator is implemented by `validation.Validator`.
type validator interface {
	Examine(context.Context, config.Set, []validation.File) (*validation.ExamineResult, error)
	Validate(context.Context, config.Set, []validation.File) (*cfgcommonpb.ValidationResult, error)
}

// ValidateConfigs validates configs. Implements configpb.ConfigsServer.
func (c Configs) ValidateConfigs(ctx context.Context, req *configpb.ValidateConfigsRequest) (*cfgcommonpb.ValidationResult, error) {
	logValidateRequest(ctx, req)
	if err := checkValidateRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err)
	}
	cs := config.Set(req.GetConfigSet())
	ctx = logging.SetField(ctx, "ConfigSet", req.GetConfigSet())

	// ACL check
	if auth.CurrentIdentity(ctx).Kind() == identity.Anonymous {
		return nil, status.Error(codes.PermissionDenied, "user must be authenticated to validate config")
	}
	switch allowed, err := acl.CanValidateConfigSet(ctx, cs); {
	case err != nil:
		logging.Errorf(ctx, "failed to check validate acls: %s", err)
		return nil, status.Errorf(codes.Internal, "error while checking acls")
	case !allowed:
		return nil, status.Errorf(codes.PermissionDenied, "%q does not have permission to validate config set %q", auth.CurrentIdentity(ctx), cs)
	}

	// Skip validation if config set does not exist
	switch result, err := datastore.Exists(ctx, &model.ConfigSet{ID: cs}); {
	case err != nil:
		logging.Errorf(ctx, "failed to check the existence of config set: %s", err)
		return nil, status.Errorf(codes.Internal, "error while checking the existence of config set %q", cs)
	case !result.All():
		return &cfgcommonpb.ValidationResult{
			Messages: []*cfgcommonpb.ValidationResult_Message{
				{
					Path:     ".",
					Severity: cfgcommonpb.ValidationResult_WARNING,
					Text:     "The config set is not registered, skipping validation",
				},
			},
		}, nil
	}

	return nil, appstatus.Errorf(codes.Unimplemented, "ValidateConfigs hasn't been implemented yet.")
}

func logValidateRequest(ctx context.Context, req *configpb.ValidateConfigsRequest) {
	reqJSON, err := protojson.Marshal(req)
	if err != nil {
		// unexpected but marshal error is fine here, just log the error.
		logging.Errorf(ctx, "failed to marshal the request to JSON: %s", err)
		return
	}
	logging.Debugf(ctx, "received validation request from %q. Request: %s", auth.CurrentIdentity(ctx), reqJSON)
}

// checkValidateRequest does a sanity check on `ValidateConfigsRequest`.
func checkValidateRequest(req *configpb.ValidateConfigsRequest) error {
	if cs := req.GetConfigSet(); cs == "" {
		return errors.New("config set is required")
	} else if err := config.Set(cs).Validate(); err != nil {
		return fmt.Errorf("invalid config set %q: %w", req.GetConfigSet(), err)
	}

	if len(req.GetFileHashes()) == 0 {
		return errors.New("must provide non-empty file_hashes")
	}
	for i, fh := range req.GetFileHashes() {
		if err := checkFileHash(fh); err != nil {
			return fmt.Errorf("file_hash[%d]: %w", i, err)
		}
	}
	return nil
}

var validSHA256Regexp = regexp.MustCompile(fmt.Sprintf(`^[0-9a-fA-F]{%d}$`, sha256.Size*2))

func checkFileHash(fileHash *configpb.ValidateConfigsRequest_FileHash) error {
	switch p := fileHash.GetPath(); {
	case p == "":
		return errors.New("path is empty")
	case path.IsAbs(p):
		return fmt.Errorf("path %q must not be absolute", p)
	default:
		for _, seg := range strings.Split(p, "/") {
			if seg == "." || seg == ".." {
				return fmt.Errorf("path %q must not contain '.' or '..' components", p)
			}
		}
	}

	switch sha256 := fileHash.GetSha256(); {
	case sha256 == "":
		return errors.New("sha256 is empty")
	case !validSHA256Regexp.MatchString(sha256):
		return fmt.Errorf("invalid sha256 hash %q", sha256)
	}
	return nil
}
