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

package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/info"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/config_service/internal/common"
)

func init() {
	addRules(&validation.Rules)
}

func addRules(r *validation.RuleSet) {
	r.Vars.Register("appid", func(ctx context.Context) (string, error) {
		if appid := info.AppID(ctx); appid != "" {
			return appid, nil
		}
		return "", fmt.Errorf("can't resolve ${appid} from context")
	})
	r.Add("exact:services/${appid}", common.ACLRegistryFilePath, validateACLsCfg)
	r.Add("exact:services/${appid}", common.ProjRegistryFilePath, validateProjectsCfg)
	r.Add("exact:services/${appid}", common.ServiceRegistryFilePath, validateServicesCfg)
	r.Add("exact:services/${appid}", common.ImportConfigFilePath, validateImportCfg)
	r.Add("exact:services/${appid}", common.SchemaConfigFilePath, validateSchemaCfg)
	r.Add(`regex:projects/[^/]+`, common.ProjMetadataFilePath, validateProjectMetadata)
	r.Add(`regex:.+`, `regex:.+\.json`, validateJSON)
}

func validateACLsCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	return nil
}

func validateServicesCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	return nil
}

func validateImportCfg(vctx *validation.Context, configSet, path string, content []byte) error {
	vctx.SetFile(path)
	cfg := &cfgcommonpb.ImportCfg{}
	if err := prototext.Unmarshal(content, cfg); err != nil {
		vctx.Errorf("invalid import proto: %s", err)
	}
	return nil
}

func validateSchemaCfg(vctx *validation.Context, configSet, path string, content []byte) error {
	vctx.SetFile(path)
	cfg := &cfgcommonpb.SchemasCfg{}
	if err := prototext.Unmarshal(content, cfg); err != nil {
		vctx.Errorf("invalid schema proto: %s", err)
		return nil
	}
	seenNames := stringset.New(len(cfg.GetSchemas()))
	for i, schema := range cfg.GetSchemas() {
		vctx.Enter("schemas #%d", i)

		vctx.Enter("name")
		switch name := schema.GetName(); {
		case name == "":
			vctx.Errorf("not specified")
		case !strings.Contains(name, ":"):
			vctx.Errorf("must contain \":\"")
		case seenNames.Has(name):
			vctx.Errorf("duplicate name: %q", name)
		default:
			segs := strings.SplitN(name, ":", 2)
			prefix, p := segs[0], segs[1] // guaranteed by the colon check before
			if cs := config.Set(prefix); prefix != "projects" && (cs.Validate() != nil || cs.Service() == "") {
				vctx.Errorf("left side of \":\" must be a service config set or \"projects\"")
			}
			vctx.Enter("right side of \":\" (path)")
			validatePath(vctx, p)
			vctx.Exit()
		}
		seenNames.Add(schema.GetName())
		vctx.Exit()

		vctx.Enter("url")
		if schema.GetUrl() == "" {
			vctx.Errorf("not specified")
		} else if u, err := url.Parse(schema.GetUrl()); err != nil {
			vctx.Errorf("invalid url: %s", err)
		} else {
			if u.Hostname() == "" {
				vctx.Errorf("hostname must be specified")
			}
			if u.Scheme != "https" {
				vctx.Errorf("scheme must be \"https\"")
			}
		}
		vctx.Exit()
		vctx.Exit()
	}
	return nil
}

func validateJSON(vctx *validation.Context, configSet, path string, content []byte) error {
	vctx.SetFile(path)
	var obj any
	if err := json.Unmarshal(content, &obj); err != nil {
		vctx.Errorf("invalid JSON: %s", err)
	}
	return nil
}

func validatePath(vctx *validation.Context, p string) {
	switch {
	case strings.TrimSpace(p) == "":
		vctx.Errorf("not specified")
	case path.IsAbs(p):
		vctx.Errorf("must not be absolute: %q", p)
	default:
		pathSegs := stringset.NewFromSlice(strings.Split(p, "/")...)
		if pathSegs.Has(".") || pathSegs.Has("..") {
			vctx.Errorf("must not contain \".\" or \"..\" components: %q", p)
		}
	}
}
