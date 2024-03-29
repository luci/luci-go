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

package ui

import (
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/config_service/internal/acl"
	configpb "go.chromium.org/luci/config_service/proto"
)

var getConfigSetMask *fieldmaskpb.FieldMask

func init() {
	var err error
	getConfigSetMask, err = fieldmaskpb.New(&configpb.ConfigSet{},
		"name", "url", "revision", "last_import_attempt", "configs")
	if err != nil {
		panic(err)
	}
}

func configSetPage(c *router.Context) error {
	csName := config.Set(strings.Trim(c.Params.ByName("ConfigSet"), "/"))
	if err := csName.Validate(); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid config set %q: %s", csName, err)
	}
	ctx := c.Request.Context()
	server := configsServer(ctx)
	cs, err := server.GetConfigSet(ctx, &configpb.GetConfigSetRequest{
		ConfigSet: string(csName),
		Fields:    getConfigSetMask,
	})
	if err != nil {
		return err
	}
	var canReimport bool
	if canReimport, err = acl.CanReimportConfigSet(ctx, csName); err != nil {
		logging.Warningf(ctx, "failed to check reimport acl: %s. continue without rendering reimport button", err)
	}
	templates.MustRender(ctx, c.Writer, "pages/config_set.html", map[string]any{
		"CanReimport": canReimport,
		"ConfigSet":   cs,
	})
	return nil
}
