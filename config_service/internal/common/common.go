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

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/config_service/internal/model"
)

const (
	// ServiceRegistryFilePath is the path of luci-config self config file where
	// it stores a list of registered services.
	ServiceRegistryFilePath = "services.cfg"
)

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
