// Copyright 2026 The LUCI Authors.
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

package cli

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/golang/mock/gomock"
	"golang.org/x/oauth2"
	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
)

func TestLogRun(t *testing.T) {
	t.Parallel()

	ftt.Run("LogRun", t, func(t *ftt.Test) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockBBClient := grpcpb.NewMockBuildsClient(ctl)
		ctx := context.Background()

		t.Run("fetchBuild requests steps and infra.logdog.hostname", func(t *ftt.Test) {
			r := &logRun{
				baseCommandRun: baseCommandRun{
					buildsClient: mockBBClient,
				},
				getBuild: &pb.GetBuildRequest{Id: 123},
			}
			expectedBuild := &pb.Build{
				Steps: []*pb.Step{{Name: "bot_update"}},
				Infra: &pb.BuildInfra{
					Logdog: &pb.BuildInfra_LogDog{
						Hostname: "logs.chromium.org",
					},
				},
			}
			mockBBClient.EXPECT().GetBuild(
				ctx,
				&pb.GetBuildRequest{
					Id:     123,
					Fields: &field_mask.FieldMask{Paths: []string{"steps", "infra.logdog.hostname"}},
				},
				gomock.Any(),
			).Return(expectedBuild, nil)

			build, err := r.fetchBuild(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, build, should.Match(expectedBuild))
		})

		t.Run("printLogs rejects missing logdogHost", func(t *ftt.Test) {
			r := &logRun{}
			logs := []*pb.Log{
				{
					Name:    "stdout",
					Url:     "logdog://logs.chromium.org/chromium/bb/+/u/step/stdout",
					ViewUrl: "https://logs.chromium.org/logs/chromium/bb/+/u/step/stdout",
				},
			}
			err := r.printLogs(ctx, "", logs)
			assert.Loosely(t, err, should.ErrLike("build does not have a logdog hostname"))
		})

		t.Run("printLogs rejects mismatched logdog host without sending requests", func(t *ftt.Test) {
			var mu sync.Mutex
			var captured int
			attacker := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				captured++
				mu.Unlock()
				w.WriteHeader(http.StatusNotFound)
			}))
			defer attacker.Close()

			attackerURL, err := url.Parse(attacker.URL)
			assert.Loosely(t, err, should.BeNil)

			base := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			client := &http.Client{
				Transport: auth.NewModifyingTransport(base, func(req *http.Request) error {
					tok := &oauth2.Token{AccessToken: "secret-token", TokenType: "Bearer"}
					tok.SetAuthHeader(req)
					return nil
				}),
			}

			r := &logRun{
				baseCommandRun: baseCommandRun{
					httpClient: client,
				},
			}
			logs := []*pb.Log{
				{
					Name:    "stdout",
					Url:     "logdog://" + attackerURL.Host + "/chromium/bb/+/u/bot_update/stdout",
					ViewUrl: "https://example.com/",
				},
			}

			err = r.printLogs(ctx, "logs.chromium.org", logs)
			assert.Loosely(t, err, should.ErrLike("does not match build logdog host"))

			mu.Lock()
			defer mu.Unlock()
			assert.Loosely(t, captured, should.Equal(0))
		})
	})
}
