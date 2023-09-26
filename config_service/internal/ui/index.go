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
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	configpb "go.chromium.org/luci/config_service/proto"
)

var mask *fieldmaskpb.FieldMask

func init() {
	var err error
	mask, err = fieldmaskpb.New(&configpb.ConfigSet{},
		"name", "url", "revision", "last_import_attempt")
	if err != nil {
		panic(err)
	}
}

func indexPage(c *router.Context) error {
	ctx := c.Request.Context()
	configSets, err := configsServer(ctx).ListConfigSets(ctx, &configpb.ListConfigSetsRequest{
		Fields: mask,
	})
	if err != nil {
		return err
	}
	templates.MustRender(ctx, c.Writer, "pages/index.html", map[string]any{
		"ConfigSets": configSets.GetConfigSets(),
	})
	return nil
}
