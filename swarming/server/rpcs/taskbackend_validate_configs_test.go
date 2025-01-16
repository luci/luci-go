// Copyright 2024 The LUCI Authors.
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

package rpcs

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateConfigs(t *testing.T) {
	t.Parallel()

	call := func(cfgMaps []map[string]any) (*bbpb.ValidateConfigsResponse, error) {
		target := "swarming://target"
		req := &bbpb.ValidateConfigsRequest{}
		for _, m := range cfgMaps {
			cfgJSON, _ := json.Marshal(m)
			cfgStruct := &structpb.Struct{}
			_ = cfgStruct.UnmarshalJSON(cfgJSON)
			req.Configs = append(req.Configs, &bbpb.ValidateConfigsRequest_ConfigContext{
				Target:     target,
				ConfigJson: cfgStruct,
			})
		}

		srv := &TaskBackend{
			BuildbucketTarget:       "swarming://target",
			BuildbucketAccount:      "ignored-in-the-test",
			DisableBuildbucketCheck: true,
		}
		return srv.ValidateConfigs(context.Background(), req)
	}

	ftt.Run("No configs", t, func(t *ftt.Test) {
		resp, err := call(nil)
		assert.NoErr(t, err)
		assert.Loosely(t, resp.ConfigErrors, should.HaveLength(0))
	})

	ftt.Run("pass", t, func(t *ftt.Test) {
		cfgMap := map[string]any{
			"priority":           30,
			"bot_ping_tolerance": 300,
			"service_account":    "sa@service-accounts.com",
			"tags": []string{
				"k1:v1",
				"k2:v2",
			},
		}
		resp, err := call([]map[string]any{cfgMap})
		assert.NoErr(t, err)
		assert.Loosely(t, resp.ConfigErrors, should.HaveLength(0))
	})

	ftt.Run("one pass, one fail ingestion, on fail validation", t, func(t *ftt.Test) {
		cfgMap1 := map[string]any{
			"priority":           30,
			"bot_ping_tolerance": 300,
			"service_account":    "sa@service-accounts.com",
			"tags": []string{
				"k1:v1",
				"k2:v2",
			},
		}
		cfgMap2 := map[string]any{
			"random_field": 300,
		}
		cfgMap3 := map[string]any{
			"priority":           300,
			"bot_ping_tolerance": 20,
			"service_account":    "none",
			"tags": []string{
				"k1:v1",
				"k2:v2",
				"invalid",
			},
		}
		resp, err := call([]map[string]any{cfgMap1, cfgMap2, cfgMap3})
		assert.NoErr(t, err)
		assert.Loosely(t, resp.ConfigErrors, should.HaveLength(5))
		for i, ed := range resp.ConfigErrors {
			if i == 0 {
				assert.Loosely(t, ed.Index, should.Equal(1))
				continue
			}
			assert.Loosely(t, ed.Index, should.Equal(2))
		}
	})
}
