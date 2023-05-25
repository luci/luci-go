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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/klauspost/compress/gzip"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

const (
	// paths for handlers
	metadataPath   = "/api/config/v1/metadata"
	validationPath = "/api/config/v1/validate"

	// Taken from
	// https://chromium.googlesource.com/infra/luci/luci-py/+/3efc60daef6bf6669f9211f63e799db47a0478c0/appengine/components/components/config/endpoint.py
	metaDataFormatVersion = "1.0"
	adminGroup            = "administrators"
)

// consumerServer implements `config.Consumer` interface that will be called
// by LUCI Config.
type consumerServer struct {
	config.UnimplementedConsumerServer

	// Rules is a rule set to use for the config validation.
	rules *validation.RuleSet

	getConfigServiceAccountFn func(context.Context) (string, error)
}

// GetMetadata implements config.Consumer.GetMetadata.
func (srv consumerServer) GetMetadata(ctx context.Context, _ *emptypb.Empty) (*config.ServiceMetadata, error) {
	if err := srv.checkCaller(ctx); err != nil {
		return nil, err
	}
	patterns, err := srv.rules.ConfigPatterns(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to collect the list of validation patterns: %s", err)
	}
	ret := &config.ServiceMetadata{}
	if len(patterns) > 0 {
		ret.ConfigPatterns = make([]*config.ConfigPattern, len(patterns))
		for i, pattern := range patterns {
			ret.ConfigPatterns[i] = &config.ConfigPattern{
				ConfigSet: pattern.ConfigSet.String(),
				Path:      pattern.Path.String(),
			}
		}
	}
	return ret, nil
}

// ValidateConfig implements config.Consumer.ValidateConfig.
func (srv consumerServer) ValidateConfig(ctx context.Context, req *config.ValidateConfigRequest) (*config.ValidateConfigResponse, error) {
	if err := srv.checkCaller(ctx); err != nil {
		return nil, err
	}
	if err := checkValidateInput(req); err != nil {
		return nil, err
	}

	result := make([][]*config.ValidateConfigResponse_Message, len(req.GetFiles().GetFiles()))
	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(8)
	for i, file := range req.GetFiles().GetFiles() {
		i, file := i, file
		eg.Go(func() error {
			content := file.GetRawContent()
			if len(content) == 0 {
				// TODO(yiwzhang): implement download from gcs.
				panic("unimplemented")
			}
			var err error
			result[i], err = srv.validateOneFile(ectx, req.GetConfigSet(), file.GetPath(), content)
			return err
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, status.Errorf(codes.Internal, "encounter internal error: %s", err)
	}

	ret := &config.ValidateConfigResponse{}
	for _, msgs := range result {
		ret.Messages = append(ret.Messages, msgs...)
	}
	return ret, nil
}

// only LUCI Config and identity in admin group is allowed to call.
func (srv consumerServer) checkCaller(ctx context.Context) error {
	configServiceAccount, err := srv.getConfigServiceAccountFn(ctx)
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

func checkValidateInput(req *config.ValidateConfigRequest) error {
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

func (srv consumerServer) validateOneFile(ctx context.Context, configSet, path string, content []byte) ([]*config.ValidateConfigResponse_Message, error) {
	vc := &validation.Context{Context: ctx}
	vc.SetFile(path)
	if err := srv.rules.ValidateConfig(vc, configSet, path, content); err != nil {
		return nil, err
	}

	var ret []*config.ValidateConfigResponse_Message
	verdict := vc.Finalize()
	switch verr, ok := verdict.(*validation.Error); {
	case verdict == nil:
	case !ok:
		ret = append(ret, &config.ValidateConfigResponse_Message{
			Path:     path,
			Severity: config.ValidateConfigResponse_ERROR,
			Text:     verdict.Error(),
		})
	case verr != nil && len(verr.Errors) > 0:
		for _, err := range verr.Errors {
			// validation.Context supports just 2 severities now,
			// but defensively default to ERROR level in unexpected cases.
			msgSeverity := config.ValidateConfigResponse_ERROR
			switch severity, ok := validation.SeverityTag.In(err); {
			case !ok:
				logging.Errorf(ctx, "unset validation.Severity in %s", err)
			case severity == validation.Warning:
				msgSeverity = config.ValidateConfigResponse_WARNING
			case severity != validation.Blocking:
				logging.Errorf(ctx, "unrecognized validation.Severity %d in %s", severity, err)
			}
			ret = append(ret, &config.ValidateConfigResponse_Message{
				Path:     path,
				Severity: msgSeverity,
				Text:     err.Error(),
			})
		}
	}
	return ret, nil
}

// InstallHandlers installs the metadata and validation handlers that use
// the given validation rules.
//
// It does not implement any authentication checks, thus the passed in
// router.MiddlewareChain should implement any necessary authentication checks.
//
// Deprecated: The handlers are called by the legacy LUCI Config service. The
// new LUCI Config service will make request to `config.Consumer` prpc service
// instead. See `consumerServer`.
func InstallHandlers(r *router.Router, base router.MiddlewareChain, rules *validation.RuleSet) {
	r.GET(metadataPath, base, metadataRequestHandler(rules))
	r.POST(validationPath, base, validationRequestHandler(rules))
}

func badRequestStatus(c context.Context, w http.ResponseWriter, msg string, err error) {
	if err != nil {
		logging.WithError(err).Warningf(c, "%s", msg)
	} else {
		logging.Warningf(c, "%s", msg)
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(msg))
}

func internalErrStatus(c context.Context, w http.ResponseWriter, msg string, err error) {
	logging.WithError(err).Errorf(c, "%s", msg)
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(msg))
}

// validationRequestHandler handles the validation request from luci-config and
// responds with the corresponding results.
func validationRequestHandler(rules *validation.RuleSet) router.Handler {
	return func(ctx *router.Context) {
		c, w, r := ctx.Context, ctx.Writer, ctx.Request

		raw := r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			logging.Infof(c, "The request is gzip compressed")
			var err error
			if raw, err = gzip.NewReader(r.Body); err != nil {
				badRequestStatus(c, w, "Failed to start decompressing gzip request body", err)
				return
			}
			defer raw.Close()
		}

		var reqBody config.ValidationRequestMessage
		switch err := json.NewDecoder(raw).Decode(&reqBody); {
		case err != nil:
			badRequestStatus(c, w, "Validation: error decoding request body", err)
			return
		case reqBody.GetConfigSet() == "":
			badRequestStatus(c, w, "Must specify the config_set of the file to validate", nil)
			return
		case reqBody.GetPath() == "":
			badRequestStatus(c, w, "Must specify the path of the file to validate", nil)
			return
		}

		vc := &validation.Context{Context: c}
		vc.SetFile(reqBody.GetPath())
		err := rules.ValidateConfig(vc, reqBody.GetConfigSet(), reqBody.GetPath(), reqBody.GetContent())
		if err != nil {
			internalErrStatus(c, w, "Validation: transient failure", err)
			return
		}

		var errors errors.MultiError
		verdict := vc.Finalize()
		if verr, _ := verdict.(*validation.Error); verr != nil {
			errors = verr.Errors
		} else if verdict != nil {
			errors = append(errors, verdict)
		}

		w.Header().Set("Content-Type", "application/json")
		var msgList []*config.ValidationResponseMessage_Message
		if len(errors) == 0 {
			logging.Infof(c, "No validation errors")
		} else {
			var errorBuffer bytes.Buffer
			for _, error := range errors {
				// validation.Context supports just 2 severities now,
				// but defensively default to ERROR level in unexpected cases.
				msgSeverity := config.ValidationResponseMessage_ERROR
				switch severity, ok := validation.SeverityTag.In(error); {
				case !ok:
					logging.Errorf(c, "unset validation.Severity in %s", error)
				case severity == validation.Warning:
					msgSeverity = config.ValidationResponseMessage_WARNING
				case severity != validation.Blocking:
					logging.Errorf(c, "unrecognized validation.Severity %d in %s", severity, error)
				}

				err := error.Error()
				msgList = append(msgList, &config.ValidationResponseMessage_Message{
					Severity: msgSeverity,
					Text:     err,
				})
				errorBuffer.WriteString("\n  " + err)
			}
			logging.Warningf(c, "Validation errors%s", errorBuffer.String())
		}
		if err := json.NewEncoder(w).Encode(config.ValidationResponseMessage{Messages: msgList}); err != nil {
			internalErrStatus(c, w, "Validation: failed to JSON encode output", err)
		}
	}
}

// metadataRequestHandler handles the metadata request from luci-config and
// responds with the necessary metadata defined by the given Validator.
func metadataRequestHandler(rules *validation.RuleSet) router.Handler {
	return func(ctx *router.Context) {
		c, w := ctx.Context, ctx.Writer

		patterns, err := rules.ConfigPatterns(c)
		if err != nil {
			internalErrStatus(c, w, "Metadata: failed to collect the list of validation patterns", err)
			return
		}

		meta := config.ServiceDynamicMetadata{
			Version:                 metaDataFormatVersion,
			SupportsGzipCompression: true,
			Validation: &config.Validator{
				Url: fmt.Sprintf("https://%s%s", ctx.Request.Host, validationPath),
			},
		}
		for _, p := range patterns {
			meta.Validation.Patterns = append(meta.Validation.Patterns, &config.ConfigPattern{
				ConfigSet: p.ConfigSet.String(),
				Path:      p.Path.String(),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(&meta); err != nil {
			internalErrStatus(c, w, "Metadata: failed to JSON encode output", err)
		}
	}
}
