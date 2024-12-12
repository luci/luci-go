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
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/validate"
)

func simpliestValidSlice(pool string) *apipb.TaskSlice {
	return &apipb.TaskSlice{
		Properties: &apipb.TaskProperties{
			GracePeriodSecs:      60,
			ExecutionTimeoutSecs: 300,
			IoTimeoutSecs:        300,
			Command:              []string{"command", "arg"},
			Dimensions: []*apipb.StringPair{
				{
					Key:   "pool",
					Value: pool,
				},
			},
		},
		ExpirationSecs: 300,
	}
}

func simpliestValidRequest(pool string) *apipb.NewTaskRequest {
	return &apipb.NewTaskRequest{
		Name:     "new",
		Priority: 40,
		TaskSlices: []*apipb.TaskSlice{
			simpliestValidSlice(pool),
		},
	}
}

func fullSlice(secretBytes []byte) *apipb.TaskSlice {
	s := &apipb.TaskSlice{
		Properties: &apipb.TaskProperties{
			GracePeriodSecs:      60,
			ExecutionTimeoutSecs: 300,
			IoTimeoutSecs:        300,
			Idempotent:           true,
			Command:              []string{"command", "arg"},
			RelativeCwd:          "cwd",
			Env: []*apipb.StringPair{
				{
					Key:   "key",
					Value: "value",
				},
			},
			EnvPrefixes: []*apipb.StringListPair{
				{
					Key: "key",
					Value: []string{
						"a/b",
					},
				},
			},
			CasInputRoot: &apipb.CASReference{
				CasInstance: "projects/project/instances/instance",
				Digest: &apipb.Digest{
					Hash:      "hash",
					SizeBytes: 1024,
				},
			},
			Outputs: []string{
				"output",
			},
			Caches: []*apipb.CacheEntry{
				{
					Name: "name",
					Path: "path",
				},
			},
			CipdInput: &apipb.CipdInput{
				Server: "https://cipd.com",
				ClientPackage: &apipb.CipdPackage{
					PackageName: "some/pkg",
					Version:     "version",
				},
				Packages: []*apipb.CipdPackage{
					{
						PackageName: "some/pkg",
						Version:     "good:tag",
						Path:        "c/d",
					},
				},
			},
			Dimensions: []*apipb.StringPair{
				{
					Key:   "pool",
					Value: "pool",
				},
			},
		},
		ExpirationSecs: 300,
	}

	if len(secretBytes) > 0 {
		s.Properties.SecretBytes = secretBytes
	}

	return s
}

func fullRequest() *apipb.NewTaskRequest {
	return &apipb.NewTaskRequest{
		Name:                 "new",
		ParentTaskId:         "60b2ed0a43023111",
		Priority:             30,
		User:                 "user",
		ServiceAccount:       "bot",
		PubsubTopic:          "projects/project/topics/topic",
		PubsubAuthToken:      "token",
		PubsubUserdata:       "userdata",
		BotPingToleranceSecs: 300,
		Realm:                "project:realm",
		Tags: []string{
			"k1:v1",
			"k2:v2",
		},
		TaskSlices: []*apipb.TaskSlice{
			fullSlice([]byte("this is a secret")),
			fullSlice(nil),
		},
		Resultdb: &apipb.ResultDBCfg{
			Enable: true,
		},
		PoolTaskTemplate: apipb.NewTaskRequest_AUTO,
	}
}

func TestValidateNewTask(t *testing.T) {
	t.Parallel()

	ftt.Run("validateNewTask", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		t.Run("empty", func(t *ftt.Test) {
			req := &apipb.NewTaskRequest{}
			_, err := validateNewTask(ctx, req)
			assert.That(t, err, should.ErrLike("name is required"))
		})

		t.Run("with_properties", func(t *ftt.Test) {
			req := &apipb.NewTaskRequest{
				Name:       "new",
				Properties: &apipb.TaskProperties{},
			}
			_, err := validateNewTask(ctx, req)
			assert.That(t, err, should.ErrLike("properties is deprecated"))
		})

		t.Run("with_expiration_secs", func(t *ftt.Test) {
			req := &apipb.NewTaskRequest{
				Name:           "new",
				ExpirationSecs: 300,
			}
			_, err := validateNewTask(ctx, req)
			assert.That(t, err, should.ErrLike("expiration_secs is deprecated"))
		})

		t.Run("parent_task_id", func(t *ftt.Test) {
			req := simpliestValidRequest("pool")
			req.ParentTaskId = "invalid"
			_, err := validateNewTask(ctx, req)
			assert.That(t, err, should.ErrLike("bad task ID"))
		})

		t.Run("priority", func(t *ftt.Test) {
			t.Run("too_small", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.Priority = -1
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("must be between 0 and 255"))
			})
			t.Run("too_big", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.Priority = 500
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("must be between 0 and 255"))
			})
		})

		t.Run("service_account", func(t *ftt.Test) {
			cases := []struct {
				sa  string
				err any
			}{
				{"", nil},
				{"none", nil},
				{"bot", nil},
				{"sa@service-accounts.com", nil},
				{"invalid", "invalid"},
			}
			for _, cs := range cases {
				t.Run(cs.sa, func(t *ftt.Test) {
					req := simpliestValidRequest("pool")
					req.ServiceAccount = cs.sa
					_, err := validateNewTask(ctx, req)
					assert.That(t, err, should.ErrLike(cs.err))
				})
			}
		})

		t.Run("pubsub_topic", func(t *ftt.Test) {
			cases := []struct {
				name  string
				topic string
				err   any
			}{
				{"empty", "", nil},
				{"valid", "projects/project/topics/topic", nil},
				{"invalid", "invalid", "does not match"},
			}
			for _, cs := range cases {
				t.Run(cs.name, func(t *ftt.Test) {
					req := simpliestValidRequest("pool")
					req.PubsubTopic = cs.topic
					_, err := validateNewTask(ctx, req)
					assert.That(t, err, should.ErrLike(cs.err))
				})
			}
		})

		t.Run("pubsub_auth_token", func(t *ftt.Test) {
			t.Run("no topic", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.PubsubAuthToken = "token"
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("pubsub_auth_token requires pubsub_topic"))
			})

		})

		t.Run("pubsub_userdata", func(t *ftt.Test) {
			t.Run("no topic", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.PubsubUserdata = "data"
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("pubsub_userdata requires pubsub_topic"))
			})
			t.Run("too_long", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.PubsubTopic = "projects/project/topics/topic"
				req.PubsubUserdata = strings.Repeat("l", maxPubsubUserDataLength+1)
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("too long"))
			})

		})

		t.Run("bot_ping_tolerance", func(t *ftt.Test) {
			t.Run("too_small", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.BotPingToleranceSecs = 20
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("must be between"))
			})
			t.Run("too_big", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.BotPingToleranceSecs = 5000
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("must be between"))
			})
		})

		t.Run("realm", func(t *ftt.Test) {
			req := simpliestValidRequest("pool")
			req.Realm = "invalid"
			_, err := validateNewTask(ctx, req)
			assert.That(t, err, should.ErrLike("invalid"))
		})

		t.Run("tags", func(t *ftt.Test) {
			t.Run("too_many", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				for i := 0; i < maxTagCount+1; i++ {
					req.Tags = append(req.Tags, "k:v")
				}
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("up to 256 tags"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.Tags = []string{"invalid"}
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("invalid"))
			})
		})

		t.Run("task_slices", func(t *ftt.Test) {
			t.Run("no_task_slices", func(t *ftt.Test) {
				req := &apipb.NewTaskRequest{
					Name: "new",
				}
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("task_slices is required"))
			})

			t.Run("secret_bytes", func(t *ftt.Test) {
				t.Run("too_long", func(t *ftt.Test) {
					req := &apipb.NewTaskRequest{
						Name: "new",
						TaskSlices: []*apipb.TaskSlice{
							{
								Properties: &apipb.TaskProperties{
									SecretBytes: []byte(strings.Repeat("a", maxSecretBytesLength+1)),
								},
							},
						},
					}
					_, err := validateNewTask(ctx, req)
					assert.That(t, err, should.ErrLike("exceeding limit"))
				})

				t.Run("unmatch", func(t *ftt.Test) {
					req := &apipb.NewTaskRequest{
						Name:     "new",
						Priority: 40,
						TaskSlices: []*apipb.TaskSlice{
							{
								Properties: &apipb.TaskProperties{
									GracePeriodSecs:      60,
									ExecutionTimeoutSecs: 300,
									IoTimeoutSecs:        300,
									Command:              []string{"command", "arg"},
									Dimensions: []*apipb.StringPair{
										{
											Key:   "pool",
											Value: "pool",
										},
									},
									SecretBytes: []byte("this is a secret"),
								},
								ExpirationSecs: 300,
							},
							{
								Properties: &apipb.TaskProperties{
									GracePeriodSecs:      60,
									ExecutionTimeoutSecs: 300,
									IoTimeoutSecs:        300,
									Command:              []string{"command", "arg"},
									Dimensions: []*apipb.StringPair{
										{
											Key:   "pool",
											Value: "pool",
										},
									},
									SecretBytes: []byte("this is another secret"),
								},
								ExpirationSecs: 300,
							},
						},
					}
					_, err := validateNewTask(ctx, req)
					assert.That(t, err, should.ErrLike("when using secret_bytes multiple times, all values must match"))
				})
			})

			t.Run("expiration_secs", func(t *ftt.Test) {
				t.Run("too_short", func(t *ftt.Test) {
					req := &apipb.NewTaskRequest{
						Name: "new",
						TaskSlices: []*apipb.TaskSlice{
							{
								Properties: &apipb.TaskProperties{},
							},
						},
					}
					_, err := validateNewTask(ctx, req)
					assert.That(t, err, should.ErrLike("must be between"))
				})

				t.Run("too_long", func(t *ftt.Test) {
					req := &apipb.NewTaskRequest{
						Name:                 "new",
						BotPingToleranceSecs: 300,
						TaskSlices: []*apipb.TaskSlice{
							{
								Properties:     &apipb.TaskProperties{},
								ExpirationSecs: maxExpirationSecs + 1,
							},
						},
					}
					_, err := validateNewTask(ctx, req)
					assert.That(t, err, should.ErrLike("must be between"))
				})
			})

			t.Run("different_pools", func(t *ftt.Test) {
				req := &apipb.NewTaskRequest{
					Name:                 "new",
					BotPingToleranceSecs: 300,
					TaskSlices: []*apipb.TaskSlice{
						simpliestValidSlice("pool1"),
						simpliestValidSlice("pool2"),
					},
				}
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("each task slice must use the same pool dimensions"))
			})

			t.Run("duplicated_slices", func(t *ftt.Test) {
				req := &apipb.NewTaskRequest{
					Name:                 "new",
					BotPingToleranceSecs: 300,
					TaskSlices: []*apipb.TaskSlice{
						simpliestValidSlice("pool1"),
						simpliestValidSlice("pool1"),
					},
				}
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("cannot request duplicate task slice"))
			})

			t.Run("task_properties", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					req := &apipb.NewTaskRequest{
						Name: "new",
						TaskSlices: []*apipb.TaskSlice{
							{
								ExpirationSecs: 600,
							},
						},
					}
					_, err := validateNewTask(ctx, req)
					assert.That(t, err, should.ErrLike("invalid properties of slice 0: required"))
				})
				t.Run("grace_period_secs", func(t *ftt.Test) {
					t.Run("too_short", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.GracePeriodSecs = 10
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("must be between"))
					})
					t.Run("too_long", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.GracePeriodSecs = maxGracePeriodSecs + 1
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("must be between"))
					})
				})

				t.Run("command", func(t *ftt.Test) {
					t.Run("empty", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Command = []string{}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("command: required"))
					})
					t.Run("too_many_args", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						for i := 0; i < maxCmdArgs+1; i++ {
							req.TaskSlices[0].Properties.Command = append(req.TaskSlices[0].Properties.Command, "arg")
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("can have up to 128 arguments"))
					})
				})

				t.Run("env", func(t *ftt.Test) {
					t.Run("too_many_keys", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						envItem := &apipb.StringPair{
							Key:   "key",
							Value: "value",
						}
						for i := 0; i < validate.MaxEnvVarCount+1; i++ {
							req.TaskSlices[0].Properties.Env = append(req.TaskSlices[0].Properties.Env, envItem)
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("can have up to 64 keys"))
					})

					t.Run("duplicate_keys", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Env = []*apipb.StringPair{
							{
								Key:   "key",
								Value: "value",
							},
							{
								Key:   "key",
								Value: "value",
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("same key cannot be specified twice"))
					})

					t.Run("key", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Env = []*apipb.StringPair{
							{
								Value: "value",
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("required"))
					})

					t.Run("value_too_long", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Env = []*apipb.StringPair{
							{
								Key:   "key",
								Value: strings.Repeat("a", validate.MaxEnvValueLength+1),
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("too long"))
					})
				})

				t.Run("env_prefixes", func(t *ftt.Test) {
					t.Run("too_many_keys", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						epItem := &apipb.StringListPair{
							Key: "key",
							Value: []string{
								"value",
							},
						}
						for i := 0; i < validate.MaxEnvVarCount+1; i++ {
							req.TaskSlices[0].Properties.EnvPrefixes = append(req.TaskSlices[0].Properties.EnvPrefixes, epItem)
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("can have up to 64 keys"))
					})

					t.Run("duplicate_keys", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.EnvPrefixes = []*apipb.StringListPair{
							{
								Key:   "key",
								Value: []string{"value"},
							},
							{
								Key:   "key",
								Value: []string{"value"},
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("same key cannot be specified twice"))
					})

					t.Run("value", func(t *ftt.Test) {
						t.Run("empty", func(t *ftt.Test) {
							req := simpliestValidRequest("pool")
							req.TaskSlices[0].Properties.EnvPrefixes = []*apipb.StringListPair{
								{
									Key: "key",
								},
							}
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("value is required"))
						})
						t.Run("with_invalid_path", func(t *ftt.Test) {
							req := simpliestValidRequest("pool")
							req.TaskSlices[0].Properties.EnvPrefixes = []*apipb.StringListPair{
								{
									Key: "key",
									Value: []string{
										"a\\b",
									},
								},
							}
							_, err := validateNewTask(ctx, req)
							assert.Loosely(t, err, should.ErrLike(`cannot contain "\\".`))
						})
					})
				})

				t.Run("cas_input_root", func(t *ftt.Test) {
					t.Run("cas_instance", func(t *ftt.Test) {
						t.Run("empty", func(t *ftt.Test) {
							req := simpliestValidRequest("pool")
							req.TaskSlices[0].Properties.CasInputRoot = &apipb.CASReference{}
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("cas_instance is required"))
						})
						t.Run("invalid", func(t *ftt.Test) {
							req := simpliestValidRequest("pool")
							req.TaskSlices[0].Properties.CasInputRoot = &apipb.CASReference{
								CasInstance: "invalid",
							}
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("should match"))
						})
					})
					t.Run("digest", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						casInputRoot := &apipb.CASReference{
							CasInstance: "projects/project/instances/instance",
						}
						req.TaskSlices[0].Properties.CasInputRoot = casInputRoot
						t.Run("empty", func(t *ftt.Test) {
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("digest: required"))
						})
						t.Run("empty_hash", func(t *ftt.Test) {
							casInputRoot.Digest = &apipb.Digest{}
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("hash is required"))
						})
						t.Run("zero_size", func(t *ftt.Test) {
							casInputRoot.Digest = &apipb.Digest{
								Hash: "hash",
							}
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("size_bytes is required"))
						})
					})
				})

				t.Run("outputs", func(t *ftt.Test) {
					t.Run("too_many", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						for i := 0; i < maxOutputCount+1; i++ {
							req.TaskSlices[0].Properties.Outputs = append(req.TaskSlices[0].Properties.Outputs, "output")
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("can have up to 512 outputs"))
					})
				})

				t.Run("caches", func(t *ftt.Test) {
					req := simpliestValidRequest("pool")
					req.TaskSlices[0].Properties.Caches = []*apipb.CacheEntry{
						{
							Name: "name1",
							Path: "a/b",
						},
						{
							Name: "name2",
							Path: "a/b",
						},
					}
					_, err := validateNewTask(ctx, req)
					assert.That(t, err, should.ErrLike(`"a/b": directory has conflicting owners: task_cache:name1[] and task_cache:name2[]`))
				})

				t.Run("cipd_input", func(t *ftt.Test) {
					t.Run("server", func(t *ftt.Test) {
						cases := []struct {
							tn     string
							server string
							err    any
						}{
							{"empty", "", "required"},
							{"too_long", strings.Repeat("a", maxCIPDServerLength+1), "too long"},
						}
						for _, cs := range cases {
							t.Run(cs.tn, func(t *ftt.Test) {
								req := simpliestValidRequest("pool")
								req.TaskSlices[0].Properties.CipdInput = &apipb.CipdInput{
									Server: cs.server,
								}
								_, err := validateNewTask(ctx, req)
								assert.That(t, err, should.ErrLike(cs.err))
							})
						}
					})
					t.Run("client_package", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.CipdInput = &apipb.CipdInput{
							Server: "https://cipd.com",
						}
						t.Run("nil", func(t *ftt.Test) {
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("required"))
						})
						t.Run("with_path", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.ClientPackage = &apipb.CipdPackage{
								Path: "a/b",
							}
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("path must be unset"))
						})
						t.Run("no name", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.ClientPackage = &apipb.CipdPackage{}
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("required"))
						})
						t.Run("invalid name", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.ClientPackage = &apipb.CipdPackage{
								PackageName: ".",
							}
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("invalid package name"))
						})
						t.Run("no_version", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.ClientPackage = &apipb.CipdPackage{
								PackageName: "some/pkg",
							}
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("required"))
						})
					})
					t.Run("packages", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.CipdInput = &apipb.CipdInput{
							Server: "https://cipd.com",
							ClientPackage: &apipb.CipdPackage{
								PackageName: "some/pkg",
								Version:     "version",
							},
						}
						t.Run("cache_path", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.Packages = []*apipb.CipdPackage{
								{
									PackageName: "some/pkg",
									Version:     "version1",
									Path:        "a/b",
								},
							}
							req.TaskSlices[0].Properties.Caches = []*apipb.CacheEntry{
								{
									Name: "name",
									Path: "a/b",
								},
							}
							_, err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike(`"a/b": directory has conflicting owners: task_cache:name[] and task_cipd_package[some/pkg:version1]`))
						})
					})
				})

				t.Run("dimensions", func(t *ftt.Test) {
					t.Run("too_many_dimensions", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						for i := 0; i < maxDimensionKeyCount+1; i++ {
							req.TaskSlices[0].Properties.Dimensions = append(
								req.TaskSlices[0].Properties.Dimensions, &apipb.StringPair{
									Key:   fmt.Sprintf("key%d", i),
									Value: "value",
								})
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("can have up to 32 keys"))
					})
					t.Run("invalid_key", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "bad key",
								Value: "value",
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("should match"))
					})

					t.Run("no_pool", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "key",
								Value: "value",
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("pool is required"))
					})
					t.Run("multiple_pool", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "pool",
								Value: "p1",
							},
							{
								Key:   "pool",
								Value: "p2",
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("pool cannot be specified more than once"))
					})
					t.Run("multiple_pool_with_ordim", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "pool",
								Value: "p1|p2",
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("pool cannot be specified more than once"))
					})
					t.Run("multiple_id", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "id",
								Value: "id1",
							},
							{
								Key:   "id",
								Value: "id2",
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("id cannot be specified more than once"))
					})
					t.Run("too_many_dimension_values", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						for i := 0; i < maxDimensionValueCount+1; i++ {
							req.TaskSlices[0].Properties.Dimensions = append(
								req.TaskSlices[0].Properties.Dimensions, &apipb.StringPair{
									Key:   "key",
									Value: "value",
								})
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("can have up to 16 values for key"))
					})
					t.Run("repeated_values", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "key",
								Value: "value",
							},
							{
								Key:   "key",
								Value: "value",
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("repeated values"))
					})
					t.Run("invalid_value", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "key",
								Value: " bad value",
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("no leading or trailing spaces"))
					})
					t.Run("invalid_or_values", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "key",
								Value: "v1| v2",
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("no leading or trailing spaces"))
					})
					t.Run("too_many_or_values", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "key1",
								Value: "v1|v2|v3",
							},
							{
								Key:   "key2",
								Value: "v4|v5|v6",
							},
						}
						_, err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("or dimensions should not be more than 8"))
					})
				})
			})
		})

		t.Run("OK", func(t *ftt.Test) {
			t.Run("without_optional_fields", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				pool, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, pool, should.Equal("pool"))
			})

			t.Run("with_all_fields", func(t *ftt.Test) {
				req := fullRequest()
				pool, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, pool, should.Equal("pool"))
			})
		})
	})
}

func TestLogNewRequest(t *testing.T) {
	ctx := memory.Use(context.Background())
	ctx = memlogger.Use(ctx)
	logs := logging.Get(ctx).(*memlogger.MemLogger)
	secret := []byte("this is a secret")
	req := &apipb.NewTaskRequest{
		TaskSlices: []*apipb.TaskSlice{
			{
				Properties: &apipb.TaskProperties{
					SecretBytes: secret,
				},
			},
		},
	}
	redacted := &apipb.NewTaskRequest{
		TaskSlices: []*apipb.TaskSlice{
			{
				Properties: &apipb.TaskProperties{
					SecretBytes: []byte("<REDACTED>"),
				},
			},
		},
	}
	logNewRequest(ctx, req)
	assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(
		logging.Info, fmt.Sprintf(`NewTaskRequest %+v`, redacted)))
	assert.Loosely(t, req.TaskSlices[0].Properties.SecretBytes, should.Match(secret))
}

func TestToTaskRequestEntities(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	state := NewMockedRequestState()
	ctx = MockRequestState(ctx, state)

	ftt.Run("toTaskRequestEntities", t, func(t *ftt.Test) {
		t.Run("high_priority", func(t *ftt.Test) {
			req := simpliestValidRequest("pool")
			req.Priority = 10
			t.Run("from_normal_user", func(t *ftt.Test) {
				ents, err := toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, ents.request.Priority, should.Equal(int64(20)))
			})
			t.Run("from_admin", func(t *ftt.Test) {
				ctx := MockRequestState(ctx, state.SetCaller(AdminFakeCaller))
				ents, err := toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, should.ErrLike(nil))
				assert.That(t, ents.request.Priority, should.Equal(int64(10)))
			})
		})

		taskRequest := func(id string) *model.TaskRequest {
			key, err := model.TaskIDToRequestKey(ctx, id)
			assert.That(t, err, should.ErrLike(nil))
			return &model.TaskRequest{
				Key:        key,
				User:       "parent_user",
				RootTaskID: "root_id",
				TaskSlices: []model.TaskSlice{
					{
						Properties: model.TaskProperties{
							Dimensions: model.TaskDimensions{
								"pool": {"pool"},
							},
						},
					},
				},
			}
		}

		taskResult := func(id string, state apipb.TaskState) *model.TaskResultSummary {
			key, _ := model.TaskIDToRequestKey(ctx, id)
			return &model.TaskResultSummary{
				Key: model.TaskResultSummaryKey(ctx, key),
				TaskResultCommon: model.TaskResultCommon{
					State: state,
				},
			}
		}
		t.Run("parent_failures", func(t *ftt.Test) {
			t.Run("parent_not_found", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.ParentTaskId = "60b2ed0a43056111"
				_, err := toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.That(t, err, should.ErrLike(`parent task "60b2ed0a43056111" not found`))
			})
			t.Run("terminate_task", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.ParentTaskId = "60b2ed0a43067111"
				key, err := model.TaskIDToRequestKey(ctx, req.ParentTaskId)
				assert.That(t, err, should.ErrLike(nil))
				pTR := &model.TaskRequest{
					Key: key,
					TaskSlices: []model.TaskSlice{
						{
							Properties: model.TaskProperties{
								Dimensions: model.TaskDimensions{
									"id": {"id"},
								},
							},
						},
					},
				}
				pTRS := taskResult(req.ParentTaskId, apipb.TaskState_RUNNING)
				assert.That(t, datastore.Put(ctx, pTR, pTRS), should.ErrLike(nil))

				_, err = toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("termination task cannot be a parent"))
			})
			t.Run("parent_has_ended", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.ParentTaskId = "60b2ed0a43078111"
				pTR := taskRequest(req.ParentTaskId)
				pTRS := taskResult(req.ParentTaskId, apipb.TaskState_COMPLETED)
				assert.That(t, datastore.Put(ctx, pTR, pTRS), should.ErrLike(nil))

				_, err := toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.That(t, err, should.ErrLike(`parent task "60b2ed0a43078111" has ended`))
			})

		})

		t.Run("apply_default_values", func(t *ftt.Test) {
			req := simpliestValidRequest("pool")
			ents, err := toTaskRequestEntities(ctx, req, "pool")
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, ents.request.ServiceAccount, should.Equal("none"))
			assert.That(t, ents.request.BotPingToleranceSecs, should.Equal(int64(1200)))
		})
		t.Run("from_full_request", func(t *ftt.Test) {
			now := time.Date(2024, time.January, 1, 2, 3, 4, 0, time.UTC)
			ctx, _ = testclock.UseTime(ctx, now)

			pID := "60b2ed0a43023111"
			pTR := taskRequest(pID)
			pTRS := taskResult(pID, apipb.TaskState_RUNNING)
			assert.That(t, datastore.Put(ctx, pTR, pTRS), should.ErrLike(nil))

			req := fullRequest()
			ents, err := toTaskRequestEntities(ctx, req, "pool")
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, ents.secretBytes, should.Match(
				&model.SecretBytes{SecretBytes: []byte("this is a secret")}))
			expectedProps := func(hasSecretBytes bool) model.TaskProperties {
				return model.TaskProperties{
					GracePeriodSecs:      int64(60),
					ExecutionTimeoutSecs: int64(300),
					IOTimeoutSecs:        int64(300),
					Idempotent:           true,
					Command:              []string{"command", "arg"},
					RelativeCwd:          "cwd",
					HasSecretBytes:       hasSecretBytes,
					CASInputRoot: model.CASReference{
						CASInstance: "projects/project/instances/instance",
						Digest: model.CASDigest{
							Hash:      "hash",
							SizeBytes: 1024,
						},
					},
					Env: model.Env{
						"key": "value",
					},
					EnvPrefixes: model.EnvPrefixes{
						"key": []string{"a/b"},
					},
					Dimensions: model.TaskDimensions{
						"pool": []string{"pool"},
					},
					Caches: []model.CacheEntry{
						{
							Name: "name",
							Path: "path",
						},
					},
					CIPDInput: model.CIPDInput{
						Server: "https://cipd.com",
						ClientPackage: model.CIPDPackage{
							PackageName: "some/pkg",
							Version:     "version",
						},
						Packages: []model.CIPDPackage{
							{
								PackageName: "some/pkg",
								Version:     "good:tag",
								Path:        "c/d",
							},
						},
					},
					Outputs: []string{
						"output",
					},
				}
			}
			expectedTR := &model.TaskRequest{
				Name:          "new",
				Created:       now,
				ParentTaskID:  datastore.NewIndexedNullable("60b2ed0a43023111"),
				Authenticated: DefaultFakeCaller,
				User:          "parent_user",
				ManualTags: []string{
					"k1:v1",
					"k2:v2",
				},
				ServiceAccount:       "bot",
				Realm:                "project:realm",
				RealmsEnabled:        true,
				Priority:             int64(30),
				PubSubTopic:          "projects/project/topics/topic",
				PubSubAuthToken:      "token",
				PubSubUserData:       "userdata",
				BotPingToleranceSecs: int64(300),
				Expiration:           now.Add(time.Duration(600) * time.Second),
				ResultDB:             model.ResultDBConfig{Enable: true},
				TaskSlices: []model.TaskSlice{
					{
						ExpirationSecs: int64(300),
						Properties:     expectedProps(true),
					},
					{
						ExpirationSecs: int64(300),
						Properties:     expectedProps(false),
					},
				},
			}
			assert.That(t, ents.request.ToProto(), should.Match(expectedTR.ToProto()))
			assert.That(t, ents.request.RootTaskID, should.Equal(pTR.RootTaskID))
		})
	})
}

func mockNewTaskRequestState() *MockedRequestState {
	state := NewMockedRequestState()
	visiblePool := state.Configs.MockPool("visible-pool", "project:visible-pool-realm")
	visiblePool.DefaultTaskRealm = "project:visible-task-realm"
	state.Configs.MockPool("visible-pool2", "project:visible-pool2-realm")
	state.MockPerm("project:visible-pool-realm", acls.PermPoolsCreateTask)
	state.MockPerm("project:visible-task-realm", acls.PermTasksCreateInRealm)
	state.MockPermWithIdentity("project:visible-task-realm",
		identity.Identity("user:sa@account.com"), acls.PermTasksActAs)
	return state
}

func TestNewTask(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	srv := TasksServer{}
	state := mockNewTaskRequestState()
	ftt.Run("perm_check_failures", t, func(t *ftt.Test) {
		t.Run("pool_not_exist", func(t *ftt.Test) {
			ctx = MockRequestState(ctx, state)
			req := simpliestValidRequest("unknown-pool")
			_, err := srv.NewTask(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike("swarming.pools.createTask"))
		})
		t.Run("pool_perm_check_fails", func(t *ftt.Test) {
			state := state.SetCaller(identity.AnonymousIdentity)
			_, err := srv.NewTask(MockRequestState(ctx, state), simpliestValidRequest("pool"))
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike("swarming.pools.createTask"))
		})

		t.Run("create_in_realm_fails", func(t *ftt.Test) {
			state := state.SetCaller(identity.Identity("user:new@example.com"))
			state.MockPerm("project:visible-pool-realm", acls.PermPoolsCreateTask)
			state.MockPerm("project:visible-pool2-realm", acls.PermPoolsCreateTask)
			ctx = MockRequestState(ctx, state)
			t.Run("no_task_realm", func(t *ftt.Test) {
				req := simpliestValidRequest("visible-pool2")
				_, err := srv.NewTask(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})
			t.Run("task_realm", func(t *ftt.Test) {
				req := simpliestValidRequest("visible-pool")
				req.Realm = "project:visible-task-realm"
				_, err := srv.NewTask(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike("swarming.tasks.createInRealm"))
			})
			t.Run("default_task_realm", func(t *ftt.Test) {
				req := simpliestValidRequest("visible-pool")
				_, err := srv.NewTask(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, req.Realm, should.Equal("project:visible-task-realm"))
				assert.That(t, err, should.ErrLike("swarming.tasks.createInRealm"))
			})
		})

		t.Run("service_account_check_fails", func(t *ftt.Test) {
			ctx = MockRequestState(ctx, state)
			req := simpliestValidRequest("visible-pool")
			req.ServiceAccount = "unknown@account.com"
			_, err := srv.NewTask(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike("swarming.tasks.actAs"))
		})
	})
}
