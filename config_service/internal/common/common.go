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

// Package common holds the shared functionalities across config_service.
package common

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
)

const (
	// ACLRegistryFilePath is the path of luci-config self config file where
	// it stores the service ACL configurations.
	ACLRegistryFilePath = "acl.cfg"

	// ProjRegistryFilePath is the path of luci-config self config file where it
	// stores a list of registered projects.
	ProjRegistryFilePath = "projects.cfg"

	// ServiceRegistryFilePath is the path of luci-config self config file where
	// it stores a list of registered services.
	ServiceRegistryFilePath = "services.cfg"

	// GSProdCfgFolder is the folder name where it stores all production configs
	// in GCS bucket.
	GSProdCfgFolder = "configs"

	// signedURLExpireDur defines how long the signed url will be active.
	signedURLExpireDur = 10 * time.Minute
)

var bucketName string
var bucketNameOnce sync.Once

// BucketName returns the global bucket name where it stores configs.
func BucketName(ctx context.Context) string {
	bucketNameOnce.Do(func() {
		bucketName = fmt.Sprintf("storage-%s", info.AppID(ctx))
	})
	return bucketName
}

// GitilesURL assembles a URL from the given gitiles location.
// Note: it doesn't validate the format of GitilesLocation.
func GitilesURL(loc *cfgcommonpb.GitilesLocation) string {
	if loc == nil || loc.Repo == "" {
		return ""
	}
	url := loc.Repo
	if loc.Ref != "" {
		url = fmt.Sprintf("%s/+/%s", url, loc.Ref)
	}
	if loc.Path != "" {
		url = fmt.Sprintf("%s/%s", url, loc.Path)
	}
	return url
}

// LoadSelfConfig reads LUCI Config self config and parse it to the provided
// message.
//
// Return model.NoSuchConfigError if the provided config file can not be found.
func LoadSelfConfig[T proto.Message](ctx context.Context, fileName string, configMsg T) error {
	file, err := model.GetLatestConfigFile(ctx, config.MustServiceSet(info.AppID(ctx)), fileName, true)
	if err != nil {
		return err
	}
	if err := prototext.Unmarshal(file.Content, configMsg); err != nil {
		return fmt.Errorf("failed to unmarshal file %q: %w", fileName, err)
	}
	return nil
}

// CreateSignedURLs create signed urls for the given GS paths.
//
// Uses the service account the service currently runs as to authorize
// and sign the urls.
func CreateSignedURLs(ctx context.Context, gsClient clients.GsClient, paths []gs.Path, method string, headers map[string]string) ([]string, error) {
	signer := auth.GetSigner(ctx)
	info, err := signer.ServiceInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get service info: %w", err)
	}
	expireAt := clock.Now(ctx).Add(signedURLExpireDur)
	var hdrs []string
	if len(headers) > 0 {
		hdrs = make([]string, 0, len(headers))
		for k, v := range headers {
			hdrs = append(hdrs, fmt.Sprintf("%s:%s", k, v))
		}
		sort.Strings(hdrs)
	}
	ret := make([]string, len(paths))
	for i, path := range paths {
		bucket, object := path.Split()
		var err error
		ret[i], err = gsClient.SignedURL(bucket, object, &storage.SignedURLOptions{
			GoogleAccessID: info.ServiceAccountName,
			SignBytes: func(b []byte) ([]byte, error) {
				_, signedBytes, err := signer.SignBytes(ctx, b)
				return signedBytes, err
			},
			Scheme:  storage.SigningSchemeV4,
			Method:  method,
			Expires: expireAt,
			Headers: hdrs,
		})
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}
