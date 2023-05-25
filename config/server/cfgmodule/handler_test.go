// Copyright 2017 The LUCI Authors.
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

package cfgmodule

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInstallHandlers(t *testing.T) {
	t.Parallel()

	Convey("Initialization of validator, validation routes and handlers", t, func() {
		rules := validation.NewRuleSet()

		r := router.New()
		rr := httptest.NewRecorder()
		host := "example.com"

		metaCall := func() *config.ServiceDynamicMetadata {
			req, err := http.NewRequest("GET", "https://"+host+metadataPath, nil)
			So(err, ShouldBeNil)
			r.ServeHTTP(rr, req)

			var resp config.ServiceDynamicMetadata
			err = json.NewDecoder(rr.Body).Decode(&resp)
			So(err, ShouldBeNil)
			return &resp
		}
		valCall := func(configSet, path, content string) *config.ValidationResponseMessage {
			respBodyJSON, err := json.Marshal(config.ValidationRequestMessage{
				ConfigSet: configSet,
				Path:      path,
				Content:   []byte(content),
			})
			So(err, ShouldBeNil)
			req, err := http.NewRequest("POST", validationPath, bytes.NewReader(respBodyJSON))
			So(err, ShouldBeNil)
			r.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				return nil
			}
			var resp config.ValidationResponseMessage
			err = json.NewDecoder(rr.Body).Decode(&resp)
			So(err, ShouldBeNil)
			return &resp
		}

		InstallHandlers(r, nil, rules)

		Convey("Basic metadataHandler call", func() {
			So(rr.Code, ShouldEqual, http.StatusOK)
			So(metaCall(), ShouldResemble, &config.ServiceDynamicMetadata{
				Version:                 metaDataFormatVersion,
				SupportsGzipCompression: true,
				Validation: &config.Validator{
					Url: fmt.Sprintf("https://%s%s", host, validationPath),
				},
			})
		})

		Convey("metadataHandler call with patterns", func() {
			rules.Add("configSet", "path", nil)
			meta := metaCall()
			So(rr.Code, ShouldEqual, http.StatusOK)
			So(meta, ShouldResemble, &config.ServiceDynamicMetadata{
				Version:                 metaDataFormatVersion,
				SupportsGzipCompression: true,
				Validation: &config.Validator{
					Url: fmt.Sprintf("https://%s%s", host, validationPath),
					Patterns: []*config.ConfigPattern{
						{
							ConfigSet: "exact:configSet",
							Path:      "exact:path",
						},
					},
				},
			})
		})

		Convey("Basic validationHandler call", func() {
			rules.Add("dead", "beef", func(ctx *validation.Context, configSet, path string, content []byte) error {
				So(string(content), ShouldEqual, "content")
				ctx.Errorf("blocking error")
				ctx.Warningf("diagnostic warning")
				return nil
			})
			valResp := valCall("dead", "beef", "content")
			So(rr.Code, ShouldEqual, http.StatusOK)
			So(valResp, ShouldResemble, &config.ValidationResponseMessage{
				Messages: []*config.ValidationResponseMessage_Message{
					{
						Text:     "in \"beef\": blocking error",
						Severity: config.ValidationResponseMessage_ERROR,
					},
					{
						Text:     "in \"beef\": diagnostic warning",
						Severity: config.ValidationResponseMessage_WARNING,
					},
				},
			})
		})

		Convey("validationHandler call with no configSet or path", func() {
			valCall("", "", "")
			So(rr.Code, ShouldEqual, http.StatusBadRequest)
			So(rr.Body.String(), ShouldEqual, "Must specify the config_set of the file to validate")
		})

		Convey("validationHandler call with no path", func() {
			valCall("dead", "", "")
			So(rr.Code, ShouldEqual, http.StatusBadRequest)
			So(rr.Body.String(), ShouldEqual, "Must specify the path of the file to validate")
		})
	})
}

func TestConsumerServer(t *testing.T) {
	t.Parallel()

	Convey("ConsumerServer", t, func() {
		const configSA = "luci-config-service@luci-config.iam.gserviceaccount.com"
		authState := &authtest.FakeState{
			Identity: "user:" + configSA,
		}
		ctx := auth.WithState(context.Background(), authState)
		rules := validation.NewRuleSet()
		srv := consumerServer{
			rules: rules,
			getConfigServiceAccountFn: func(ctx context.Context) (string, error) {
				return configSA, nil
			},
		}

		Convey("Check caller", func() {
			Convey("Allow LUCI Config service account", func() {
				_, err := srv.GetMetadata(ctx, &emptypb.Empty{})
				So(err, ShouldBeNil)
			})
			Convey("Allow Admin group", func() {
				authState := &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{adminGroup},
				}
				ctx = auth.WithState(context.Background(), authState)
				_, err := srv.GetMetadata(ctx, &emptypb.Empty{})
				So(err, ShouldBeNil)
			})
			Convey("Disallow", func() {
				Convey("Non-admin users", func() {
					authState = &authtest.FakeState{
						Identity: "user:someone@example.com",
					}
				})
				Convey("Anonymous", func() {
					authState = &authtest.FakeState{
						Identity: identity.AnonymousIdentity,
					}
				})
				ctx = auth.WithState(context.Background(), authState)
				_, err := srv.GetMetadata(ctx, &emptypb.Empty{})
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
			})
		})

		Convey("GetMetadata", func() {
			rules.Add("configSet", "path", nil)
			res, err := srv.GetMetadata(ctx, &emptypb.Empty{})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &config.ServiceMetadata{
				ConfigPatterns: []*config.ConfigPattern{
					{
						ConfigSet: "exact:configSet",
						Path:      "exact:path",
					},
				},
			})
		})
	})
}
