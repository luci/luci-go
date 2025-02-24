// Copyright 2025 The LUCI Authors.
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

package botapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/caching"
	minterpb "go.chromium.org/luci/tokenserver/api/minter/v1"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/model"
)

func TestTaskTokens(t *testing.T) {
	t.Parallel()

	const (
		testBotID     = "test-bot"
		testTaskSA    = "task-sa@example.com"
		testTaskRealm = "task-project:task-realm"
		testTaskName  = "task name"
		testToken     = "minted-token"
	)

	var testTokenExpiry = time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission("user:"+testTaskSA, testTaskRealm, acls.PermTasksActAs),
			),
		})
		ctx = cryptorand.MockForTest(ctx, 1) // for generating task IDs

		mockedMinter := &mockedMinter{
			tokenValue:  testToken,
			tokenExpiry: testTokenExpiry,
		}

		srv := &BotAPIServer{
			cfg:     cfgtest.MockConfigs(ctx, cfgtest.NewMockedConfigs()),
			project: "swarming-proj",
			version: "swarming-ver",
			tokenServerClient: func(ctx context.Context, realm string) (minterpb.TokenMinterClient, error) {
				assert.That(t, realm, should.Equal(testTaskRealm))
				return mockedMinter, nil
			},
		}

		mockTask := func(serviceAccount, realm string) string {
			ent := &model.TaskRequest{
				Key:            model.NewTaskRequestKey(ctx),
				ServiceAccount: serviceAccount,
				Realm:          realm,
				Name:           testTaskName,
			}
			assert.NoErr(t, datastore.Put(ctx, ent))
			return model.RequestKeyToTaskID(ent.Key, model.AsRunResult)
		}

		t.Run("OAuth token", func(t *ftt.Test) {
			call := func(currentTaskID string, req *TokenRequest) (*TokenResponse, error) {
				r, err := srv.OAuthToken(ctx, req, &botsrv.Request{
					Session: &internalspb.Session{
						BotId: testBotID,
					},
					CurrentTaskID: currentTaskID,
				})
				if err != nil {
					return nil, err
				}
				return r.(*TokenResponse), nil
			}

			t.Run("OK: empty, bot, none", func(t *ftt.Test) {
				for _, acc := range []string{"", "bot", "none"} {
					taskID := mockTask(acc, testTaskRealm)
					r, err := call(taskID, &TokenRequest{
						AccountID: "task",
						TaskID:    taskID,
						Scopes:    []string{"some-scope"},
					})
					assert.NoErr(t, err)

					expected := acc
					if expected == "" {
						expected = "none"
					}
					assert.That(t, r, should.Match(&TokenResponse{
						ServiceAccount: expected,
					}))
				}
			})

			t.Run("OK: actual account", func(t *ftt.Test) {
				taskID := mockTask(testTaskSA, testTaskRealm)
				r, err := call(taskID, &TokenRequest{
					AccountID: "task",
					TaskID:    taskID,
					Scopes:    []string{"some-scope1", "some-scope2"},
				})
				assert.NoErr(t, err)

				assert.That(t, r, should.Match(&TokenResponse{
					ServiceAccount: testTaskSA,
					AccessToken:    testToken,
					Expiry:         testTokenExpiry.Unix(),
				}))

				assert.That(t, mockedMinter.req, should.Match(&minterpb.MintServiceAccountTokenRequest{
					TokenKind:           minterpb.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
					ServiceAccount:      testTaskSA,
					Realm:               testTaskRealm,
					OauthScope:          []string{"some-scope1", "some-scope2"},
					MinValidityDuration: int64(minTokenTTL.Seconds()),
					AuditTags: []string{
						"swarming:bot_id:" + testBotID,
						"swarming:task_id:" + taskID,
						"swarming:task_name:" + testTaskName,
						"swarming:trace_id:00000000000000000000000000000000",
						"swarming:service_version:swarming-proj/swarming-ver",
					},
				}))
			})

			t.Run("No scopes", func(t *ftt.Test) {
				taskID := mockTask(testTaskSA, testTaskRealm)
				_, err := call(taskID, &TokenRequest{
					AccountID: "task",
					TaskID:    taskID,
				})

				assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`"scopes" are required`))
			})

			t.Run("Unexpected audience", func(t *ftt.Test) {
				taskID := mockTask(testTaskSA, testTaskRealm)
				_, err := call(taskID, &TokenRequest{
					AccountID: "task",
					TaskID:    taskID,
					Scopes:    []string{"some-scope"},
					Audience:  "huh",
				})

				assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`"audience" must not be used`))
			})

			t.Run("Unrecognized account ID", func(t *ftt.Test) {
				taskID := mockTask(testTaskSA, testTaskRealm)
				_, err := call(taskID, &TokenRequest{
					AccountID: "huh",
					TaskID:    taskID,
					Scopes:    []string{"some-scope"},
				})

				assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`"account_id" must be either "system" or "task"`))
			})

			t.Run("No task ID", func(t *ftt.Test) {
				_, err := call(testTaskSA, &TokenRequest{
					AccountID: "task",
					Scopes:    []string{"some-scope"},
				})

				assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`"task_id" is required`))
			})

			t.Run("Malformed task ID", func(t *ftt.Test) {
				_, err := call(testTaskSA, &TokenRequest{
					AccountID: "task",
					TaskID:    "huh",
					Scopes:    []string{"some-scope"},
				})

				assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`bad task ID`))
			})

			t.Run("Wrong task ID", func(t *ftt.Test) {
				taskID1 := mockTask(testTaskSA, testTaskRealm)
				taskID2 := mockTask(testTaskSA, testTaskRealm)

				_, err := call(taskID1, &TokenRequest{
					AccountID: "task",
					TaskID:    taskID2,
					Scopes:    []string{"some-scope"},
				})

				assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`the bot is not executing this task`))
			})

			t.Run("Revoked permission", func(t *ftt.Test) {
				taskID := mockTask(testTaskSA, "some:unknown-realm")
				_, err := call(taskID, &TokenRequest{
					AccountID: "task",
					TaskID:    taskID,
					Scopes:    []string{"some-scope"},
				})

				assert.That(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`the service account "task-sa@example.com" doesn't have permission "swarming.tasks.actAs" in the realm "some:unknown-realm"`))
			})
		})

		t.Run("ID token", func(t *ftt.Test) {
			call := func(currentTaskID string, req *TokenRequest) (*TokenResponse, error) {
				r, err := srv.IDToken(ctx, req, &botsrv.Request{
					Session: &internalspb.Session{
						BotId: testBotID,
					},
					CurrentTaskID: currentTaskID,
				})
				if err != nil {
					return nil, err
				}
				return r.(*TokenResponse), nil
			}

			// Only test code paths which are different from the OAuth token case.

			t.Run("OK: actual account", func(t *ftt.Test) {
				taskID := mockTask(testTaskSA, testTaskRealm)
				r, err := call(taskID, &TokenRequest{
					AccountID: "task",
					TaskID:    taskID,
					Audience:  "audience",
				})
				assert.NoErr(t, err)

				assert.That(t, r, should.Match(&TokenResponse{
					ServiceAccount: testTaskSA,
					IDToken:        testToken,
					Expiry:         testTokenExpiry.Unix(),
				}))

				assert.That(t, mockedMinter.req, should.Match(&minterpb.MintServiceAccountTokenRequest{
					TokenKind:           minterpb.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN,
					ServiceAccount:      testTaskSA,
					Realm:               testTaskRealm,
					IdTokenAudience:     "audience",
					MinValidityDuration: int64(minTokenTTL.Seconds()),
					AuditTags: []string{
						"swarming:bot_id:" + testBotID,
						"swarming:task_id:" + taskID,
						"swarming:task_name:" + testTaskName,
						"swarming:trace_id:00000000000000000000000000000000",
						"swarming:service_version:swarming-proj/swarming-ver",
					},
				}))
			})

			t.Run("No audience", func(t *ftt.Test) {
				taskID := mockTask(testTaskSA, testTaskRealm)
				_, err := call(taskID, &TokenRequest{
					AccountID: "task",
					TaskID:    taskID,
				})

				assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`"audience" is required`))
			})

			t.Run("Unexpected scopes", func(t *ftt.Test) {
				taskID := mockTask(testTaskSA, testTaskRealm)
				_, err := call(taskID, &TokenRequest{
					AccountID: "task",
					TaskID:    taskID,
					Audience:  "audience",
					Scopes:    []string{"scope"},
				})

				assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`"scopes" must not be used`))
			})
		})
	})
}

func TestSystemTokens(t *testing.T) {
	t.Parallel()

	// Only test code paths which are different from the "task" token case.

	const (
		testBotID       = "test-bot"
		testSystemSA    = "system-sa@example.com"
		testAccessToken = "minted-token"
	)

	var (
		testTime        = time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
		testTokenExpiry = testTime.Add(time.Hour)
	)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx, _ = testclock.UseTime(ctx, testTime)

		mockedTokenProvider := &mockedTokenProvider{
			accessToken: testAccessToken,
			expiry:      testTokenExpiry,
		}

		ctx = auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
			cfg.ActorTokensProvider = mockedTokenProvider
			return cfg
		})

		srv := &BotAPIServer{}

		t.Run("OAuth token", func(t *ftt.Test) {
			call := func(systemAccount string, req *TokenRequest) (*TokenResponse, error) {
				r, err := srv.OAuthToken(ctx, req, &botsrv.Request{
					Session: &internalspb.Session{
						BotId: testBotID,
						BotConfig: &internalspb.BotConfig{
							SystemServiceAccount: systemAccount,
						},
					},
				})
				if err != nil {
					return nil, err
				}
				return r.(*TokenResponse), nil
			}

			t.Run("OK: empty, bot, none", func(t *ftt.Test) {
				for _, acc := range []string{"", "bot", "none"} {
					r, err := call(acc, &TokenRequest{
						AccountID: "system",
						Scopes:    []string{"some-scope"},
					})
					assert.NoErr(t, err)

					expected := acc
					if expected == "" {
						expected = "none"
					}
					assert.That(t, r, should.Match(&TokenResponse{
						ServiceAccount: expected,
					}))
				}
			})

			t.Run("OK: actual account", func(t *ftt.Test) {
				r, err := call(testSystemSA, &TokenRequest{
					AccountID: "system",
					Scopes:    []string{"scope1", "scope2"},
				})
				assert.NoErr(t, err)

				assert.That(t, r, should.Match(&TokenResponse{
					ServiceAccount: testSystemSA,
					AccessToken:    testAccessToken,
					Expiry:         testTokenExpiry.Unix(),
				}))

				assert.That(t, mockedTokenProvider.serviceAccount, should.Equal(testSystemSA))
				assert.That(t, mockedTokenProvider.scopes, should.Match([]string{"scope1", "scope2"}))
				assert.That(t, mockedTokenProvider.audience, should.Equal(""))
				assert.That(t, mockedTokenProvider.delegates, should.Match([]string(nil)))
			})
		})

		t.Run("ID token", func(t *ftt.Test) {
			call := func(systemAccount string, req *TokenRequest) (*TokenResponse, error) {
				r, err := srv.IDToken(ctx, req, &botsrv.Request{
					Session: &internalspb.Session{
						BotId: testBotID,
						BotConfig: &internalspb.BotConfig{
							SystemServiceAccount: systemAccount,
						},
					},
				})
				if err != nil {
					return nil, err
				}
				return r.(*TokenResponse), nil
			}

			r, err := call(testSystemSA, &TokenRequest{
				AccountID: "system",
				Audience:  "audience",
			})
			assert.NoErr(t, err)

			assert.That(t, r, should.Match(&TokenResponse{
				ServiceAccount: testSystemSA,
				IDToken:        mockedTokenProvider.idToken, // generated
				Expiry:         testTokenExpiry.Unix(),
			}))

			assert.That(t, mockedTokenProvider.serviceAccount, should.Equal(testSystemSA))
			assert.That(t, mockedTokenProvider.scopes, should.Match([]string(nil)))
			assert.That(t, mockedTokenProvider.audience, should.Equal("audience"))
			assert.That(t, mockedTokenProvider.delegates, should.Match([]string(nil)))
		})
	})
}

func TestTokenServerClient(t *testing.T) {
	t.Parallel()

	db := authtest.NewFakeDB(
		authtest.MockTokenServiceURL("https://fake-token-server.example.com"),
	)

	ctx := auth.ModifyConfig(context.Background(), func(cfg auth.Config) auth.Config {
		cfg.DBProvider = db.AsProvider()
		cfg.AnonymousTransport = func(ctx context.Context) http.RoundTripper { return http.DefaultTransport }
		return cfg
	})

	// Doesn't crash. Good enough.
	_, err := tokenServerClient(ctx, "some:realm")
	assert.NoErr(t, err)
}

type mockedMinter struct {
	minterpb.TokenMinterClient // implement the interface by panicking

	req         *minterpb.MintServiceAccountTokenRequest
	err         error
	tokenValue  string
	tokenExpiry time.Time
}

func (m *mockedMinter) MintServiceAccountToken(ctx context.Context, in *minterpb.MintServiceAccountTokenRequest, opts ...grpc.CallOption) (*minterpb.MintServiceAccountTokenResponse, error) {
	m.req = in
	if m.err != nil {
		return nil, m.err
	}
	return &minterpb.MintServiceAccountTokenResponse{
		Token:  m.tokenValue,
		Expiry: timestamppb.New(m.tokenExpiry),
	}, nil
}

type mockedTokenProvider struct {
	serviceAccount string
	scopes         []string
	audience       string
	delegates      []string

	accessToken string
	idToken     string

	expiry time.Time
}

func (m *mockedTokenProvider) GenerateAccessToken(ctx context.Context, serviceAccount string, scopes, delegates []string) (*oauth2.Token, error) {
	m.serviceAccount = serviceAccount
	m.scopes = scopes
	m.audience = ""
	m.delegates = delegates
	return &oauth2.Token{AccessToken: m.accessToken, Expiry: m.expiry}, nil
}

func (m *mockedTokenProvider) GenerateIDToken(ctx context.Context, serviceAccount, audience string, delegates []string) (string, error) {
	m.serviceAccount = serviceAccount
	m.scopes = nil
	m.audience = audience
	m.delegates = delegates

	// The caller parses the ID token to get the expiry. Generate a fake one.
	body, _ := json.Marshal(&openid.IDToken{Exp: m.expiry.Unix()})
	b64bdy := base64.RawURLEncoding.EncodeToString(body)
	m.idToken = "mockedhdr." + b64bdy + "." + "mockedsig"
	return m.idToken, nil
}
