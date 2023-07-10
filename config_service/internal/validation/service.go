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
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
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
	service  *model.Service
	gsClient clients.GsClient
	cs       config.Set
	files    []File
}

func (sv *serviceValidator) validate(ctx context.Context) (*cfgcommonpb.ValidationResult, error) {
	if sv.service.Name == info.AppID(ctx) {
		panic("implement")
	}
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport %w", err)
	}
	switch {
	case sv.service.Info.GetServiceEndpoint() != "":
		endpoint := sv.service.Info.GetServiceEndpoint()
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
	case sv.service.Info.GetMetadataUrl() != "":
		// TODO(yiwzhang): support legacy protocol
		panic("unimplemented")
	default:
		return nil, errors.New("expected either service_endpoint or metadata_url to be non-empty")
	}
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
	urls, err := common.CreateSignedURLs(ctx, sv.gsClient, gsPaths, http.MethodGet, map[string]string{
		"Accept-Encoding": "gzip",
	})
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
