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

package validation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/klauspost/compress/gzip"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
)

// serviceValidator calls external service to validate the config or
// validate locally for config the service itself it is interested in.
type serviceValidator struct {
	service     *model.Service
	gsClient    clients.GsClient
	selfRuleSet *validation.RuleSet
	cs          config.Set
	files       []File
}

func (sv *serviceValidator) validate(ctx context.Context) (*cfgcommonpb.ValidationResult, error) {
	switch {
	case sv.service.Info.GetId() == info.AppID(ctx):
		return sv.validateAgainstSelfRules(ctx)
	case sv.service.Info.GetHostname() != "":
		tr, err := auth.GetRPCTransport(ctx, auth.AsSelf)
		if err != nil {
			return nil, fmt.Errorf("failed to create transport %w", err)
		}
		endpoint := sv.service.Info.GetHostname()
		prpcClient := &prpc.Client{
			C:    &http.Client{Transport: tr},
			Host: endpoint,
		}
		if strings.HasPrefix(endpoint, "127.0.0.1") { // testing
			prpcClient.Options = &prpc.Options{Insecure: true}
		}
		client := cfgcommonpb.NewConsumerClient(prpcClient)
		req, err := sv.prepareRequest(ctx)
		if err != nil {
			return nil, err
		}
		return client.ValidateConfigs(ctx, req)
	case sv.service.LegacyMetadata != nil:
		return sv.validateInLegacyProtocol(ctx)
	default:
		return nil, fmt.Errorf("service is not %s; it also doesn't provide either hostname or metadata_url for validation", sv.service.Info.GetId())
	}
}

// validateAgainstSelfRules validates config files against the rules
// registered to the current service (i.e. LUCI Config itself).
func (sv *serviceValidator) validateAgainstSelfRules(ctx context.Context) (*cfgcommonpb.ValidationResult, error) {
	var msgs []*cfgcommonpb.ValidationResult_Message
	var msgsMu sync.Mutex
	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(8)

	for _, file := range sv.files {
		eg.Go(func() (err error) {
			path := file.GetPath()
			content, err := file.GetRawContent(ectx)
			if err != nil {
				return err
			}
			vc := &validation.Context{Context: ectx}
			vc.SetFile(path)
			if err := sv.selfRuleSet.ValidateConfig(vc, string(sv.cs), path, content); err != nil {
				return err
			}
			var vErr *validation.Error
			switch err := vc.Finalize(); {
			case errors.As(err, &vErr):
				msgsMu.Lock()
				msgs = append(msgs, vErr.ToValidationResultMsgs(ctx)...)
				msgsMu.Unlock()
			case err != nil:
				msgsMu.Lock()
				msgs = append(msgs, &cfgcommonpb.ValidationResult_Message{
					Path:     path,
					Severity: cfgcommonpb.ValidationResult_ERROR,
					Text:     err.Error(),
				})
				msgsMu.Unlock()
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return &cfgcommonpb.ValidationResult{
		Messages: msgs,
	}, nil
}

func (sv *serviceValidator) prepareRequest(ctx context.Context) (*cfgcommonpb.ValidateConfigsRequest, error) {
	// This needs to be optimized if it becomes a common pattern that one config
	// file will be validated by multiple services. Right now each service
	// generates signed url for each file that will be included in the validation
	// request. If a file will be validated against N services, N signed urls will
	// be generated instead of one.
	gsPaths := make([]gs.Path, len(sv.files))
	for i, f := range sv.files {
		gsPaths[i] = f.GetGSPath()
	}
	urls, err := common.CreateSignedURLs(ctx, sv.gsClient, gsPaths, http.MethodGet, nil)
	if err != nil {
		return nil, err
	}
	req := &cfgcommonpb.ValidateConfigsRequest{
		ConfigSet: string(sv.cs),
		Files: &cfgcommonpb.ValidateConfigsRequest_Files{
			Files: make([]*cfgcommonpb.ValidateConfigsRequest_File, len(sv.files)),
		},
	}
	for i, url := range urls {
		req.Files.Files[i] = &cfgcommonpb.ValidateConfigsRequest_File{
			Path: sv.files[i].GetPath(),
			Content: &cfgcommonpb.ValidateConfigsRequest_File_SignedUrl{
				SignedUrl: url,
			},
		}
	}
	return req, nil
}

type legacyValidationRequest struct {
	ConfigSet string `json:"config_set"`
	Path      string `json:"path"`
	Content   string `json:"content"` // base64 encoded
}

type legacyValidationResponse struct {
	Messages []legacyValidationResponseMessage `json:"messages"`
}

type legacyValidationResponseMessage struct {
	Severity cfgcommonpb.ValidationResult_Severity
	Text     string
}

// UnmarshalJSON unmarshal json string to legacyValidationResponseMessage.
func (msg *legacyValidationResponseMessage) UnmarshalJSON(b []byte) error {
	var objMap map[string]*json.RawMessage
	if err := json.Unmarshal(b, &objMap); err != nil {
		return err
	}
	if text, ok := objMap["text"]; ok {
		if err := json.Unmarshal(*text, &msg.Text); err != nil {
			return err
		}
	}
	if rawSev, ok := objMap["severity"]; ok {
		var sevInt int32
		var sevStr string
		switch {
		case json.Unmarshal(*rawSev, &sevInt) == nil:
			if _, ok := cfgcommonpb.ValidationResult_Severity_name[sevInt]; !ok {
				return fmt.Errorf("unrecognized severity integer %d", sevInt)
			}
			msg.Severity = cfgcommonpb.ValidationResult_Severity(sevInt)
		case json.Unmarshal(*rawSev, &sevStr) == nil:
			sevVal, ok := cfgcommonpb.ValidationResult_Severity_value[sevStr]
			if !ok {
				return fmt.Errorf("unrecognized severity string %q", sevStr)
			}
			msg.Severity = cfgcommonpb.ValidationResult_Severity(sevVal)
		default:
			return fmt.Errorf("unrecognized severity \"%s\"", *rawSev)
		}
	}
	return nil
}

// validateInLegacyProtocol validates all files of the `serviceValidator`
// against a service using legacy protocol.
func (sv *serviceValidator) validateInLegacyProtocol(ctx context.Context) (*cfgcommonpb.ValidationResult, error) {
	var allMsgs []*cfgcommonpb.ValidationResult_Message
	var msgsMu sync.Mutex
	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(8)

	for _, file := range sv.files {
		eg.Go(func() error {
			msgs, err := sv.validateFileLegacy(ectx, file)
			if err != nil {
				return err
			}
			msgsMu.Lock()
			allMsgs = append(allMsgs, msgs...)
			msgsMu.Unlock()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return &cfgcommonpb.ValidationResult{
		Messages: allMsgs,
	}, nil
}

// validateFileLegacy validates a file against service using legacy protocol.
//
// It is an HTTP POST request. The request and response formats are defined by
// `legacyValidationRequest` and `legacyValidationResponse` respectively. It
// also respects the `support_gzip_compression` setting in the service config.
// It will compress any payload over 512KiB if enabled.
func (sv *serviceValidator) validateFileLegacy(ctx context.Context, file File) ([]*cfgcommonpb.ValidationResult_Message, error) {
	ctx = logging.SetFields(ctx, logging.Fields{
		"Service": sv.service.Name,
		"File":    file.GetPath(),
	})
	headers := map[string]string{
		"Content-Type": "application/json; charset=utf-8",
		"User-Agent":   info.AppID(ctx),
	}
	content, err := file.GetRawContent(ctx)
	if err != nil {
		return nil, err
	}
	req := legacyValidationRequest{
		ConfigSet: string(sv.cs),
		Path:      file.GetPath(),
		Content:   base64.StdEncoding.EncodeToString(content),
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal the request to JSON: %w", err)
	}
	var buf bytes.Buffer
	if sv.service.LegacyMetadata.GetSupportsGzipCompression() && len(payload) > 512*1024 {
		gzipWriter := gzip.NewWriter(&buf)
		if _, err := gzipWriter.Write(payload); err != nil {
			_ = gzipWriter.Close()
			return nil, fmt.Errorf("failed to gzip compress the request: %w", err)
		}
		if err := gzipWriter.Close(); err != nil {
			return nil, fmt.Errorf("failed to close gzip writer: %w", err)
		}
		headers["Content-Encoding"] = "gzip"
	} else {
		buf = *bytes.NewBuffer(payload)
	}
	url := sv.service.LegacyMetadata.GetValidation().GetUrl()
	if url == "" {
		panic(fmt.Errorf("expect non-empty legacy validation url for service %q", sv.service.Name))
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{}
	if jwtAud := sv.service.Info.GetJwtAuth().GetAudience(); jwtAud != "" {
		if client.Transport, err = common.GetSelfSignedJWTTransport(ctx, jwtAud); err != nil {
			return nil, err
		}
	} else {
		if client.Transport, err = auth.GetRPCTransport(ctx, auth.AsSelf); err != nil {
			return nil, fmt.Errorf("failed to create transport %w", err)
		}
	}
	logging.Debugf(ctx, "POST %s Content-Length: %d", url, buf.Len())
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to %s: %w", url, err)
	}
	return sv.parseLegacyResponse(ctx, resp, url, file)
}

func (sv *serviceValidator) parseLegacyResponse(ctx context.Context, resp *http.Response, url string, file File) ([]*cfgcommonpb.ValidationResult_Message, error) {
	defer func() { _ = resp.Body.Close() }()
	switch body, err := io.ReadAll(resp.Body); {
	case err != nil:
		return nil, fmt.Errorf("failed to read the response from %s: %w", url, err)
	case resp.StatusCode != http.StatusOK:
		logging.Errorf(ctx, "validating against %s using legacy protocol fails with status code: %d. Full response body:\n\n%s", sv.service.Name, resp.StatusCode, body)
		return nil, fmt.Errorf("%s returns %d", url, resp.StatusCode)
	case len(body) == 0:
		return nil, nil
	default:
		validationResponse := legacyValidationResponse{}
		if err := json.Unmarshal(body, &validationResponse); err != nil {
			logging.Errorf(ctx, "failed to unmarshal legacy validation response: %s; Full response body: %s", err, body)
			return nil, fmt.Errorf("failed to unmarshal response from %s: %w", url, err)
		}
		ret := make([]*cfgcommonpb.ValidationResult_Message, 0, len(validationResponse.Messages))
		for _, msg := range validationResponse.Messages {
			if msg.Severity == cfgcommonpb.ValidationResult_UNKNOWN {
				logging.Errorf(ctx, "severity not provided; full response from %s: %q", url, body)
				continue
			}
			ret = append(ret, &cfgcommonpb.ValidationResult_Message{
				Path:     file.GetPath(),
				Severity: msg.Severity,
				Text:     msg.Text,
			})
		}
		return ret, nil
	}
}
