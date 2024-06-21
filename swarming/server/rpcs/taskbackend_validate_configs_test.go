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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey("No configs", t, func() {
		resp, err := call(nil)
		So(err, ShouldBeNil)
		So(resp.ConfigErrors, ShouldHaveLength, 0)
	})

	Convey("pass", t, func() {
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
		So(err, ShouldBeNil)
		So(resp.ConfigErrors, ShouldHaveLength, 0)
	})

	Convey("one pass, one fail ingestion, on fail validation", t, func() {
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
		So(err, ShouldBeNil)
		So(resp.ConfigErrors, ShouldHaveLength, 5)
		for i, ed := range resp.ConfigErrors {
			if i == 0 {
				So(ed.Index, ShouldEqual, 1)
				continue
			}
			So(ed.Index, ShouldEqual, 2)
		}
	})
}

func TestSubValidations(t *testing.T) {
	t.Parallel()

	Convey("priority", t, func() {
		Convey("too small", func() {
			err := validatePriority(-1)
			So(err, ShouldErrLike, "invalid priority -1, must be between 0 and 255")
		})

		Convey("too big", func() {
			err := validatePriority(256)
			So(err, ShouldErrLike, "invalid priority 256, must be between 0 and 255")
		})
	})

	Convey("bot_ping_tolerance", t, func() {
		Convey("too small", func() {
			err := validateBotPingTolerance(int64(30))
			So(err, ShouldErrLike, "invalid bot_ping_tolerance 30, must be between 60 and 1200")
		})

		Convey("too big", func() {
			err := validateBotPingTolerance(int64(2000))
			So(err, ShouldErrLike, "invalid bot_ping_tolerance 2000, must be between 60 and 1200")
		})
	})

	Convey("service account", t, func() {
		Convey("too long", func() {
			sa := strings.Repeat("l", maxServiceAccountLength+1)
			err := validateServiceAccount(sa)
			So(err, ShouldErrLike, "service account too long")
		})

		Convey("invalid", func() {
			err := validateServiceAccount("invalid")
			So(err, ShouldErrLike, "invalid service account")
		})
	})

	Convey("tag", t, func() {
		Convey("invalid tag", func() {
			cases := []string{
				"tag",
				"tag:",
				":tag",
			}

			for _, t := range cases {
				err := validateTag(t)
				So(err, ShouldErrLike, "tag must be in key:value form")
			}
		})
	})
}
