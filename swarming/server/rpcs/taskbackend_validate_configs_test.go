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
	"strings"
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
		assert.Loosely(t, err, should.BeNil)
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
		assert.Loosely(t, err, should.BeNil)
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
		assert.Loosely(t, err, should.BeNil)
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

func TestSubValidations(t *testing.T) {
	t.Parallel()

	ftt.Run("priority", t, func(t *ftt.Test) {
		t.Run("too small", func(t *ftt.Test) {
			err := validatePriority(-1)
			assert.Loosely(t, err, should.ErrLike("invalid priority -1, must be between 0 and 255"))
		})

		t.Run("too big", func(t *ftt.Test) {
			err := validatePriority(256)
			assert.Loosely(t, err, should.ErrLike("invalid priority 256, must be between 0 and 255"))
		})
	})

	ftt.Run("bot_ping_tolerance", t, func(t *ftt.Test) {
		t.Run("too small", func(t *ftt.Test) {
			err := validateBotPingTolerance(int64(30))
			assert.Loosely(t, err, should.ErrLike("invalid bot_ping_tolerance 30, must be between 60 and 1200"))
		})

		t.Run("too big", func(t *ftt.Test) {
			err := validateBotPingTolerance(int64(2000))
			assert.Loosely(t, err, should.ErrLike("invalid bot_ping_tolerance 2000, must be between 60 and 1200"))
		})
	})

	ftt.Run("service account", t, func(t *ftt.Test) {
		t.Run("too long", func(t *ftt.Test) {
			sa := strings.Repeat("l", maxServiceAccountLength+1)
			err := validateServiceAccount(sa)
			assert.Loosely(t, err, should.ErrLike("service account too long"))
		})

		t.Run("invalid", func(t *ftt.Test) {
			err := validateServiceAccount("invalid")
			assert.Loosely(t, err, should.ErrLike("invalid service account"))
		})
	})

	ftt.Run("tag", t, func(t *ftt.Test) {
		t.Run("invalid tag", func(t *ftt.Test) {
			cases := []string{
				"tag",
				"tag:",
				":tag",
			}

			for _, tc := range cases {
				err := validateTag(tc)
				assert.Loosely(t, err, should.ErrLike("tag must be in key:value form"))
			}
		})
	})
}
