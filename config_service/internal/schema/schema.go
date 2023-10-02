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

// Package schema handles config schema lookup
package schema

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.chromium.org/luci/common/logging"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/config_service/internal/common"
)

// InstallHandler installs handler for schema redirection.
func InstallHandler(r *router.Router) {
	r.GET("/schemas/*SchemaName", nil, func(c *router.Context) {
		ctx := c.Request.Context()
		schemaName := strings.Trim(c.Params.ByName("SchemaName"), "/")
		switch url, err := lookupSchemaURL(ctx, schemaName); {
		case err != nil:
			logging.Errorf(ctx, "failed to find schema url for %q: %s", schemaName, err)
			http.Error(c.Writer, fmt.Sprintf("failed to find corresponding schema url for %q", schemaName), http.StatusInternalServerError)
		case url == "":
			http.Error(c.Writer, fmt.Sprintf("%q has no corresponding schema url", schemaName), http.StatusNotFound)
		default:
			http.Redirect(c.Writer, c.Request, url, http.StatusFound)
		}
	})
}

func lookupSchemaURL(ctx context.Context, name string) (string, error) {
	schemaCfg := &cfgcommonpb.SchemasCfg{}
	if err := common.LoadSelfConfig(ctx, common.SchemaConfigFilePath, schemaCfg); err != nil {
		return "", err
	}
	for _, schema := range schemaCfg.GetSchemas() {
		if schema.GetName() == name {
			return schema.Url, nil
		}
	}
	return "", nil
}
