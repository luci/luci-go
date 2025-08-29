// Copyright 2017 The LUCI Authors.
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

package cfgmodule

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	cfgpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/srvhttp"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
)

// Members of this group can call the validation endpoint directly.
const adminGroup = "administrators"

// ConsumerServer implements `cfgpb.Consumer` interface that will be called
// by LUCI Config.
type ConsumerServer struct {
	cfgpb.UnimplementedConsumerServer
	// Rules is a rule set to use for the config validation.
	Rules *validation.RuleSet
	// GetConfigServiceAccountFn returns a function that can fetch the service
	// account of the LUCI Config service. It is used by ACL checking.
	GetConfigServiceAccountFn func(context.Context) (string, error)
}

// GetMetadata implements cfgpb.Consumer.GetMetadata.
func (srv *ConsumerServer) GetMetadata(ctx context.Context, _ *emptypb.Empty) (*cfgpb.ServiceMetadata, error) {
	if err := srv.checkCaller(ctx); err != nil {
		return nil, err
	}
	patterns, err := srv.Rules.ConfigPatterns(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to collect the list of validation patterns: %s", err)
	}
	ret := &cfgpb.ServiceMetadata{}
	if len(patterns) > 0 {
		ret.ConfigPatterns = make([]*cfgpb.ConfigPattern, len(patterns))
		for i, pattern := range patterns {
			ret.ConfigPatterns[i] = &cfgpb.ConfigPattern{
				ConfigSet: pattern.ConfigSet.String(),
				Path:      pattern.Path.String(),
			}
		}
	}
	return ret, nil
}

// ValidateConfigs implements cfgpb.Consumer.ValidateConfigs.
func (srv *ConsumerServer) ValidateConfigs(ctx context.Context, req *cfgpb.ValidateConfigsRequest) (*cfgpb.ValidationResult, error) {
	if err := srv.checkCaller(ctx); err != nil {
		return nil, err
	}
	if err := checkValidateInput(req); err != nil {
		return nil, err
	}

	result := make([][]*cfgpb.ValidationResult_Message, len(req.GetFiles().GetFiles()))
	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(8)
	for i, file := range req.GetFiles().GetFiles() {
		eg.Go(func() error {
			var content []byte
			switch file.GetContent().(type) {
			case *cfgpb.ValidateConfigsRequest_File_RawContent:
				content = file.GetRawContent()
			case *cfgpb.ValidateConfigsRequest_File_SignedUrl:
				var err error
				if content, err = config.DownloadConfigFromSignedURL(ectx, srvhttp.DefaultClient(ctx), file.GetSignedUrl()); err != nil {
					return fmt.Errorf("failed to download file %s from the signed url: %w", file.Path, err)
				}
			default:
				panic(fmt.Errorf("unrecognized file content type: %T", file.GetContent()))
			}
			var err error
			result[i], err = srv.validateOneFile(ectx, req.GetConfigSet(), file.GetPath(), content)
			return err
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, status.Errorf(codes.Internal, "encounter internal error: %s", err)
	}

	// Flatten the messages
	ret := &cfgpb.ValidationResult{}
	for _, msgs := range result {
		ret.Messages = append(ret.Messages, msgs...)
	}
	return ret, nil
}

// only LUCI Config and identity in admin group is allowed to call.
func (srv *ConsumerServer) checkCaller(ctx context.Context) error {
	configServiceAccount, err := srv.GetConfigServiceAccountFn(ctx)
	if err != nil {
		logging.Errorf(ctx, "Failed to get LUCI Config service account: %s", err)
		return status.Errorf(codes.Internal, "failed to get LUCI Config service account")
	}
	caller := auth.CurrentIdentity(ctx)
	if caller.Kind() == identity.User && caller.Value() == configServiceAccount {
		return nil
	}
	switch admin, err := auth.IsMember(ctx, adminGroup); {
	case err != nil:
		logging.Errorf(ctx, "Failed to check ACL: %s", err)
		return status.Errorf(codes.Internal, "failed to check ACL")
	case admin:
		return nil
	}
	return status.Errorf(codes.PermissionDenied, "%q is not authorized", caller)
}

func checkValidateInput(req *cfgpb.ValidateConfigsRequest) error {
	switch {
	case req.GetConfigSet() == "":
		return status.Errorf(codes.InvalidArgument, "must specify the config_set of the file to validate")
	case len(req.GetFiles().GetFiles()) == 0:
		return status.Errorf(codes.InvalidArgument, "must provide at least 1 file to validate")
	}
	for i, file := range req.GetFiles().GetFiles() {
		if file.GetPath() == "" {
			return status.Errorf(codes.InvalidArgument, "must specify path for file[%d]", i)
		}
		if file.GetRawContent() == nil && file.GetSignedUrl() == "" {
			return status.Errorf(codes.InvalidArgument, "must either provide raw_content or signed_url for file %q", file.GetPath())
		}
	}
	return nil
}

func (srv *ConsumerServer) validateOneFile(ctx context.Context, configSet, path string, content []byte) ([]*cfgpb.ValidationResult_Message, error) {
	vc := &validation.Context{Context: ctx}
	vc.SetFile(path)
	if err := srv.Rules.ValidateConfig(vc, configSet, path, content); err != nil {
		return nil, err
	}
	var vErr *validation.Error
	switch err := vc.Finalize(); {
	case errors.As(err, &vErr):
		return vErr.ToValidationResultMsgs(ctx), nil
	case err != nil:
		return []*cfgpb.ValidationResult_Message{
			{
				Path:     path,
				Severity: cfgpb.ValidationResult_ERROR,
				Text:     err.Error(),
			},
		}, nil
	default:
		return nil, nil
	}
}
