// Copyright 2022 The LUCI Authors.
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

// Package oauth contains methods to work with oauth endpoint.
package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/impl/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOAuthServing(t *testing.T) {
	testTS := time.Date(2021, time.August, 16, 15, 20, 0, 0, time.UTC)
	testClientID := "test-client-id"
	testAdditionalClientIDs := []string{
		"additional-client-id-0",
		"additional-client-id-1",
	}
	testClientSecret := "test-client-secret"
	testTokenServer := "https://token-server.example.com"
	testPrimaryURL := "test-primary-url"

	testGlobalConfig := &model.AuthGlobalConfig{
		AuthVersionedEntityMixin: model.AuthVersionedEntityMixin{
			ModifiedTS:    testTS,
			ModifiedBy:    "user:someone@example.com",
			AuthDBRev:     2,
			AuthDBPrevRev: 1,
		},
		Kind:                     "AuthGlobalConfig",
		ID:                       "root",
		OAuthClientID:            testClientID,
		OAuthAdditionalClientIDs: testAdditionalClientIDs,
		OAuthClientSecret:        testClientSecret,
		TokenServerURL:           testTokenServer,
	}

	testAuthReplicationState := func(ctx context.Context) *model.AuthReplicationState {
		return &model.AuthReplicationState{
			Kind:       "AuthReplicationState",
			ID:         "self",
			Parent:     model.RootKey(ctx),
			AuthDBRev:  2,
			ModifiedTS: testTS,
			PrimaryURL: testPrimaryURL,
		}
	}

	legacyCall := func(ctx context.Context) ([]byte, error) {
		rw := httptest.NewRecorder()

		rctx := &router.Context{
			Writer:  rw,
			Request: (&http.Request{}).WithContext(ctx),
		}

		if err := HandleLegacyOAuthEndpoint(rctx); err != nil {
			return nil, err
		}
		return rw.Body.Bytes(), nil
	}

	t.Parallel()

	type OAuthJSON struct {
		TokenServerURL      string   `json:"token_server_url"`
		ClientSecret        string   `json:"client_not_so_secret"`
		AdditionalClientIDs []string `json:"additional_client_ids"`
		ClientID            string   `json:"client_id"`
		PrimaryURL          string   `json:"primary_url"`
	}

	Convey("Testing legacy endpoint with JSON response", t, func() {
		ctx := memory.Use(context.Background())

		Convey("AuthReplicationState entity not found", func() {
			So(datastore.Put(ctx, testGlobalConfig), ShouldBeNil)
			_, err := legacyCall(ctx)
			So(err, ShouldHaveGRPCStatus, codes.Internal)
			So(err.Error(), ShouldContainSubstring, "no Replication State entity found in datastore.")
		})

		Convey("AuthGlobalConfig entity not found", func() {
			So(datastore.Put(ctx, testAuthReplicationState(ctx)), ShouldBeNil)
			_, err := legacyCall(ctx)
			So(err, ShouldHaveGRPCStatus, codes.Internal)
			So(err.Error(), ShouldContainSubstring, "no Global Config entity found in datastore.")
		})

		Convey("OK", func() {
			So(datastore.Put(ctx,
				testGlobalConfig,
				testAuthReplicationState(ctx)), ShouldBeNil)
			actualBlob, err := legacyCall(ctx)
			So(err, ShouldBeNil)

			actualJSON := &OAuthJSON{}
			expectedJSON := &OAuthJSON{
				TokenServerURL:      testTokenServer,
				ClientSecret:        testClientSecret,
				AdditionalClientIDs: testAdditionalClientIDs,
				ClientID:            testClientID,
				PrimaryURL:          testPrimaryURL,
			}
			So(json.Unmarshal(actualBlob, actualJSON), ShouldBeNil)
			So(actualJSON, ShouldResemble, expectedJSON)
		})

	})
}
