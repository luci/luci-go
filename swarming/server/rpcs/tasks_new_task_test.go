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
	"math/rand"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/data/rand/mathrand"
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
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
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
					Key: "ep",
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
				Server: "https://cipd.example.com",
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
				assert.That(t, err, should.ErrLike("must be between 1 and 255"))
			})
			t.Run("too_big", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.Priority = 500
				_, err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("must be between 1 and 255"))
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
						Name:     "new",
						Priority: 30,
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
					Priority:             30,
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
					Priority:             30,
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
						Name:     "new",
						Priority: 30,
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
					t.Run("client_package", func(t *ftt.Test) {
						req := simpliestValidRequest("pool")
						req.TaskSlices[0].Properties.CipdInput = &apipb.CipdInput{
							Server: "https://cipd.example.com",
						}
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
							Server: "https://cipd.example.com",
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
				assert.NoErr(t, err)
				assert.That(t, pool, should.Equal("pool"))
			})

			t.Run("with_all_fields", func(t *ftt.Test) {
				req := fullRequest()
				pool, err := validateNewTask(ctx, req)
				assert.NoErr(t, err)
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
	ctx = mathrand.Set(ctx, rand.New(rand.NewSource(123)))

	state := NewMockedRequestState()
	poolCfg := state.Configs.MockPool("pool", "project:pool-realm")
	poolCfg.TaskDeploymentScheme = &configpb.Pool_TaskTemplateDeploymentInline{
		TaskTemplateDeploymentInline: &configpb.TaskTemplateDeployment{
			Prod: &configpb.TaskTemplate{},
		},
	}
	ctx = MockRequestState(ctx, state)

	ftt.Run("toTaskRequestEntities", t, func(t *ftt.Test) {
		t.Run("high_priority", func(t *ftt.Test) {
			req := simpliestValidRequest("pool")
			req.Priority = 10
			t.Run("from_normal_user", func(t *ftt.Test) {
				ents, err := toTaskRequestEntities(ctx, req, "pool")
				assert.NoErr(t, err)
				assert.That(t, ents.request.Priority, should.Equal(int64(20)))
			})
			t.Run("from_admin", func(t *ftt.Test) {
				ctx := MockRequestState(ctx, state.SetCaller(AdminFakeCaller))
				ents, err := toTaskRequestEntities(ctx, req, "pool")
				assert.NoErr(t, err)
				assert.That(t, ents.request.Priority, should.Equal(int64(10)))
			})
		})

		taskRequest := func(id string) *model.TaskRequest {
			key, err := model.TaskIDToRequestKey(ctx, id)
			assert.NoErr(t, err)
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
				assert.NoErr(t, err)
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
				assert.NoErr(t, datastore.Put(ctx, pTR, pTRS))

				_, err = toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("termination task cannot be a parent"))
			})
			t.Run("parent_has_ended", func(t *ftt.Test) {
				req := simpliestValidRequest("pool")
				req.ParentTaskId = "60b2ed0a43078111"
				pTR := taskRequest(req.ParentTaskId)
				pTRS := taskResult(req.ParentTaskId, apipb.TaskState_COMPLETED)
				assert.NoErr(t, datastore.Put(ctx, pTR, pTRS))

				_, err := toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.That(t, err, should.ErrLike(`parent task "60b2ed0a43078111" has ended`))
			})

		})

		t.Run("apply_default_values", func(t *ftt.Test) {
			req := simpliestValidRequest("pool")
			ents, err := toTaskRequestEntities(ctx, req, "pool")
			assert.NoErr(t, err)
			assert.That(t, ents.request.ServiceAccount, should.Equal("none"))
			assert.That(t, ents.request.BotPingToleranceSecs, should.Equal(int64(1200)))
		})

		t.Run("apply_task_template", func(t *ftt.Test) {
			now := time.Date(2024, time.January, 1, 2, 3, 4, 0, time.UTC)
			ctx, _ = testclock.UseTime(ctx, now)
			req := fullRequest()
			req.ParentTaskId = ""

			t.Run("conflicts_in_cache_name", func(t *ftt.Test) {
				poolCfg.TaskDeploymentScheme = &configpb.Pool_TaskTemplateDeploymentInline{
					TaskTemplateDeploymentInline: &configpb.TaskTemplateDeployment{
						Prod: &configpb.TaskTemplate{
							Cache: []*configpb.TaskTemplate_CacheEntry{
								{
									Name: "name",
									Path: "path2",
								},
							},
						},
					},
				}
				ctx := MockRequestState(ctx, state)
				_, err := toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, should.ErrLike(
					`request.cache "name" conflicts with pool's template`))
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})

			t.Run("conflicts_in_cache_path", func(t *ftt.Test) {
				poolCfg.TaskDeploymentScheme = &configpb.Pool_TaskTemplateDeploymentInline{
					TaskTemplateDeploymentInline: &configpb.TaskTemplateDeployment{
						Prod: &configpb.TaskTemplate{
							Cache: []*configpb.TaskTemplate_CacheEntry{
								{
									Name: "name1",
									Path: "path",
								},
							},
						},
					},
				}
				ctx := MockRequestState(ctx, state)
				_, err := toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, should.ErrLike(
					`"path": directory has conflicting owners: `+
						`task_cache:name[] and task_template_cache:name1[]`))
			})

			t.Run("conflicts_in_cipd_packages", func(t *ftt.Test) {
				poolCfg.TaskDeploymentScheme = &configpb.Pool_TaskTemplateDeploymentInline{
					TaskTemplateDeploymentInline: &configpb.TaskTemplateDeployment{
						Prod: &configpb.TaskTemplate{
							CipdPackage: []*configpb.TaskTemplate_CipdPackage{
								{
									Pkg:     "template/pkg",
									Version: "version",
									Path:    "c/d",
								},
							},
						},
					},
				}
				ctx := MockRequestState(ctx, state)
				_, err := toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, should.ErrLike(
					`"c/d": directory has conflicting owners: `+
						`task_cipd_packages[some/pkg:good:tag] and `+
						`task_template_cipd_packages[template/pkg:version]`))
			})

			t.Run("conflicts_in_template_cache_cipd_packages", func(t *ftt.Test) {
				poolCfg.TaskDeploymentScheme = &configpb.Pool_TaskTemplateDeploymentInline{
					TaskTemplateDeploymentInline: &configpb.TaskTemplateDeployment{
						Prod: &configpb.TaskTemplate{
							Cache: []*configpb.TaskTemplate_CacheEntry{
								{
									Name: "name1",
									Path: "c",
								},
							},
						},
					},
				}
				ctx := MockRequestState(ctx, state)
				_, err := toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, should.ErrLike(
					`task_cipd_packages[some/pkg:good:tag] uses "c/d", `+
						`which conflicts with task_template_cache:name1[] using "c"`))
			})

			t.Run("conflicts_in_env", func(t *ftt.Test) {
				poolCfg.TaskDeploymentScheme = &configpb.Pool_TaskTemplateDeploymentInline{
					TaskTemplateDeploymentInline: &configpb.TaskTemplateDeployment{
						Prod: &configpb.TaskTemplate{
							Env: []*configpb.TaskTemplate_Env{
								{
									Var:   "key",
									Value: "b",
									Prefix: []string{
										"e/f",
									},
								},
							},
						},
					},
				}
				ctx := MockRequestState(ctx, state)
				_, err := toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, should.ErrLike(
					`request.env "key" conflicts with pool's template`))
			})
			t.Run("conflicts_in_env_prefix", func(t *ftt.Test) {
				poolCfg.TaskDeploymentScheme = &configpb.Pool_TaskTemplateDeploymentInline{
					TaskTemplateDeploymentInline: &configpb.TaskTemplateDeployment{
						Prod: &configpb.TaskTemplate{
							Env: []*configpb.TaskTemplate_Env{
								{
									Var:   "ep",
									Value: "b",
									Prefix: []string{
										"e/f",
									},
								},
							},
						},
					},
				}
				ctx := MockRequestState(ctx, state)
				_, err := toTaskRequestEntities(ctx, req, "pool")
				assert.That(t, err, should.ErrLike(
					`request.env_prefix "ep" conflicts with pool's template`))
			})

			t.Run("pass", func(t *ftt.Test) {
				poolCfg.TaskDeploymentScheme = &configpb.Pool_TaskTemplateDeploymentInline{
					TaskTemplateDeploymentInline: &configpb.TaskTemplateDeployment{
						Prod: &configpb.TaskTemplate{
							Cache: []*configpb.TaskTemplate_CacheEntry{
								{
									Name: "template_c",
									Path: "c/template",
								},
							},
							CipdPackage: []*configpb.TaskTemplate_CipdPackage{
								{
									Pkg:     "template/pkg",
									Version: "prod",
									Path:    "p/template",
								},
							},
							Env: []*configpb.TaskTemplate_Env{
								{
									Var:   "key",
									Value: "b",
									Prefix: []string{
										"e/f",
									},
									Soft: true,
								},
							},
						},
						Canary: &configpb.TaskTemplate{
							Cache: []*configpb.TaskTemplate_CacheEntry{
								{
									Name: "template_c",
									Path: "c/template",
								},
							},
							CipdPackage: []*configpb.TaskTemplate_CipdPackage{
								{
									Pkg:     "template/pkg",
									Version: "canary",
									Path:    "p/template",
								},
							},
							Env: []*configpb.TaskTemplate_Env{
								{
									Var:   "ep",
									Value: "b",
									Prefix: []string{
										"e/f",
									},
									Soft: true,
								},
							},
						},
						CanaryChance: 1000,
					},
				}
				ctx := MockRequestState(ctx, state)

				expectedTags := func(template string) []string {
					return []string{
						"authenticated:user:test@example.com",
						"k1:v1",
						"k2:v2",
						"pool:pool",
						"priority:30",
						"realm:project:realm",
						"service_account:bot",
						fmt.Sprintf("swarming.pool.task_template:%s", template),
						fmt.Sprintf("swarming.pool.version:%s", State(ctx).Config.VersionInfo.Revision),
						"user:user",
					}
				}

				expectedEnv := func(template string) []*apipb.StringPair {
					if template == "canary" {
						return []*apipb.StringPair{
							{
								Key:   "ep",
								Value: "b",
							},
							{
								Key:   "key",
								Value: "value",
							},
						}
					}
					return fullSlice(nil).Properties.Env
				}

				expectedEnvPrefix := func(template string) []*apipb.StringListPair {
					switch template {
					case "none":
						return fullSlice(nil).Properties.EnvPrefixes
					case "canary":
						return []*apipb.StringListPair{
							{
								Key: "ep",
								Value: []string{
									"a/b",
									"e/f",
								},
							},
						}
					case "prod":
						return []*apipb.StringListPair{
							{
								Key: "ep",
								Value: []string{
									"a/b",
								},
							},
							{
								Key: "key",
								Value: []string{
									"e/f",
								},
							},
						}
					}
					return nil
				}

				expectedCaches := func(template string) []*apipb.CacheEntry {
					if template == "none" {
						return fullSlice(nil).Properties.Caches
					}
					return []*apipb.CacheEntry{
						{
							Name: "name",
							Path: "path",
						},
						{
							Name: "template_c",
							Path: "c/template",
						},
					}
				}

				expectedPackages := func(template string) []*apipb.CipdPackage {
					if template == "none" {
						return fullSlice(nil).Properties.CipdInput.Packages
					}

					if template == "prod" {
						return []*apipb.CipdPackage{
							{
								PackageName: "some/pkg",
								Version:     "good:tag",
								Path:        "c/d",
							},
							{
								PackageName: "template/pkg",
								Version:     "prod",
								Path:        "p/template",
							},
						}
					}
					return []*apipb.CipdPackage{
						{
							PackageName: "some/pkg",
							Version:     "good:tag",
							Path:        "c/d",
						},
						{
							PackageName: "template/pkg",
							Version:     "canary",
							Path:        "p/template",
						},
					}
				}

				testCases := []struct {
					poolTaskTemplate apipb.NewTaskRequest_PoolTaskTemplateField
					template         string
				}{
					{
						poolTaskTemplate: apipb.NewTaskRequest_AUTO,
						template:         "prod",
					},
					{
						poolTaskTemplate: apipb.NewTaskRequest_CANARY_PREFER,
						template:         "canary",
					},
					{
						poolTaskTemplate: apipb.NewTaskRequest_CANARY_NEVER,
						template:         "prod",
					},
					{
						poolTaskTemplate: apipb.NewTaskRequest_SKIP,
						template:         "none",
					},
				}

				for _, tc := range testCases {
					t.Run(apipb.NewTaskRequest_PoolTaskTemplateField_name[int32(tc.poolTaskTemplate)], func(t *ftt.Test) {
						req := fullRequest()
						req.PoolTaskTemplate = tc.poolTaskTemplate
						req.ParentTaskId = ""
						ents, err := toTaskRequestEntities(ctx, req, "pool")
						assert.NoErr(t, err)
						res := ents.request.ToProto()
						assert.That(t, res.Tags, should.Match(expectedTags(tc.template)))

						props := res.GetTaskSlices()[0].GetProperties()
						assert.That(t, props.GetCaches(), should.Match(expectedCaches(tc.template)))
						assert.That(t, props.GetCipdInput().GetPackages(),
							should.Match(expectedPackages(tc.template)))
						assert.That(t, props.GetEnv(), should.Match(expectedEnv(tc.template)))
						assert.That(t, props.GetEnvPrefixes(),
							should.Match(expectedEnvPrefix(tc.template)))
					})
				}

			})
		})

		t.Run("apply_default_cipd", func(t *ftt.Test) {
			now := time.Date(2024, time.January, 1, 2, 3, 4, 0, time.UTC)
			ctx, _ = testclock.UseTime(ctx, now)

			pID := "60b2ed0a43023111"
			pTR := taskRequest(pID)
			pTRS := taskResult(pID, apipb.TaskState_RUNNING)
			assert.NoErr(t, datastore.Put(ctx, pTR, pTRS))

			req := fullRequest()
			req.TaskSlices[0].Properties.CipdInput = nil

			ents, err := toTaskRequestEntities(ctx, req, "pool")
			assert.NoErr(t, err)
			assert.That(t, ents.request.TaskSlices[0].Properties.CIPDInput.Server, should.Equal(cfgtest.MockedCIPDServer))
			assert.That(t, ents.request.TaskSlices[0].Properties.CIPDInput.ClientPackage.PackageName, should.Equal("client/pkg"))
			assert.That(t, ents.request.TaskSlices[0].Properties.CIPDInput.ClientPackage.Version, should.Equal("latest"))
		})

		t.Run("from_full_request", func(t *ftt.Test) {
			now := time.Date(2024, time.January, 1, 2, 3, 4, 0, time.UTC)
			ctx, _ = testclock.UseTime(ctx, now)

			pID := "60b2ed0a43023111"
			pTR := taskRequest(pID)
			pTRS := taskResult(pID, apipb.TaskState_RUNNING)
			assert.NoErr(t, datastore.Put(ctx, pTR, pTRS))

			req := fullRequest()
			req.TaskSlices[0].Properties.Dimensions = append(req.TaskSlices[0].Properties.Dimensions, &apipb.StringPair{
				Key:   "multiple_value",
				Value: "a|b|c",
			})

			ents, err := toTaskRequestEntities(ctx, req, "pool")
			assert.NoErr(t, err)
			assert.That(t, ents.secretBytes, should.Match(
				&model.SecretBytes{SecretBytes: []byte("this is a secret")}))
			expectedProps := func(hasSecretBytes bool, extraDims map[string][]string) model.TaskProperties {
				dims := model.TaskDimensions{
					"pool": []string{"pool"},
				}
				for k, v := range extraDims {
					dims[k] = v
				}
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
						"ep": []string{"a/b"},
					},
					Dimensions: dims,
					Caches: []model.CacheEntry{
						{
							Name: "name",
							Path: "path",
						},
					},
					CIPDInput: model.CIPDInput{
						Server: "https://cipd.example.com",
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
				Tags: []string{
					"authenticated:user:test@example.com",
					"k1:v1",
					"k2:v2",
					"multiple_value:a",
					"multiple_value:b",
					"multiple_value:c",
					"parent_task_id:60b2ed0a43023111",
					"pool:pool",
					"priority:30",
					"realm:project:realm",
					"service_account:bot",
					"swarming.pool.task_template:prod",
					fmt.Sprintf("swarming.pool.version:%s", State(ctx).Config.VersionInfo.Revision),
					"user:parent_user",
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
						Properties:     expectedProps(true, map[string][]string{"multiple_value": {"a|b|c"}}),
					},
					{
						ExpirationSecs: int64(300),
						Properties:     expectedProps(false, nil),
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
	srv := TasksServer{
		SwarmingProject:    "swarming",
		TaskLifecycleTasks: tasks.MockTQTasks(),
	}
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

	ftt.Run("evaluate_only", t, func(t *ftt.Test) {
		ctx = MockRequestState(ctx, state)
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)
		req := simpliestValidRequest("visible-pool")
		req.PoolTaskTemplate = apipb.NewTaskRequest_SKIP
		req.EvaluateOnly = true
		res, err := srv.NewTask(ctx, req)
		assert.NoErr(t, err)
		assert.That(t, res.TaskId, should.Equal(""))
		var empty *apipb.TaskResultResponse
		assert.That(t, res.TaskResult, should.Match(empty))
		props := &apipb.TaskProperties{
			GracePeriodSecs:      60,
			ExecutionTimeoutSecs: 300,
			IoTimeoutSecs:        300,
			Command:              []string{"command", "arg"},
			Dimensions: []*apipb.StringPair{
				{
					Key:   "pool",
					Value: "visible-pool",
				},
			},
			CipdInput: &apipb.CipdInput{
				Server: cfgtest.MockedCIPDServer,
				ClientPackage: &apipb.CipdPackage{
					PackageName: "client/pkg",
					Version:     "latest",
				},
			},
		}
		expected := &apipb.TaskRequestResponse{
			Name:                 "new",
			Priority:             40,
			ExpirationSecs:       300,
			BotPingToleranceSecs: 1200,
			Properties:           props,
			Tags: []string{
				"authenticated:user:test@example.com",
				"pool:visible-pool",
				"priority:40",
				"realm:project:visible-task-realm",
				"service_account:none",
				"swarming.pool.task_template:none",
				fmt.Sprintf("swarming.pool.version:%s", State(ctx).Config.VersionInfo.Revision),
				"user:none",
			},
			CreatedTs:      timestamppb.New(now),
			Authenticated:  string(DefaultFakeCaller),
			ServiceAccount: "none",
			Realm:          "project:visible-task-realm",
			Resultdb:       &apipb.ResultDBCfg{},
			TaskSlices: []*apipb.TaskSlice{
				{
					ExpirationSecs: 300,
					Properties:     props,
				},
			},
		}
		assert.That(t, res.Request, should.Match(expected))
	})

	ftt.Run("dedup_by_task_request_id", t, func(t *ftt.Test) {
		ctx = MockRequestState(ctx, state)

		id := "65aba3a3e6b99310"
		reqKey, err := model.TaskIDToRequestKey(ctx, id)
		assert.NoErr(t, err)
		tr := &model.TaskRequest{
			Key: reqKey,
		}
		trs := &model.TaskResultSummary{
			Key: model.TaskResultSummaryKey(ctx, reqKey),
			TaskResultCommon: model.TaskResultCommon{
				State: apipb.TaskState_COMPLETED,
			},
		}
		tri := &model.TaskRequestID{
			Key:    model.TaskRequestIDKey(ctx, "user:test@example.com:request-id"),
			TaskID: id,
		}
		assert.NoErr(t, datastore.Put(ctx, tr, trs, tri))

		req := simpliestValidRequest("visible-pool")
		req.PoolTaskTemplate = apipb.NewTaskRequest_SKIP
		req.RequestUuid = "request-id"

		res, err := srv.NewTask(ctx, req)
		assert.NoErr(t, err)
		assert.That(t, res.TaskId, should.Equal(id))
		assert.That(t, res.TaskResult, should.Match(trs.ToProto()))
	})

	ftt.Run("retry_on_id_collision", t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(ctx, time.Date(2020, time.January, 1, 2, 3, 4, 0, time.UTC))
		ctx = MockRequestState(ctx, state)
		ctx = cryptorand.MockForTest(ctx, 0)
		ctx = memlogger.Use(ctx)
		logs := logging.Get(ctx).(*memlogger.MemLogger)

		id := "4977a91bc012fa10"
		reqKey, err := model.TaskIDToRequestKey(ctx, id)
		assert.NoErr(t, err)
		tr := &model.TaskRequest{
			Key: reqKey,
		}
		trs := &model.TaskResultSummary{
			Key: model.TaskResultSummaryKey(ctx, reqKey),
			TaskResultCommon: model.TaskResultCommon{
				State: apipb.TaskState_COMPLETED,
			},
		}
		assert.NoErr(t, datastore.Put(ctx, tr, trs))

		req := simpliestValidRequest("visible-pool")
		req.PoolTaskTemplate = apipb.NewTaskRequest_SKIP

		res, err := srv.NewTask(ctx, req)
		assert.NoErr(t, err)
		assert.That(t, res.TaskId, should.NotEqual(id))
		assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(
			logging.Info, "Created the task after 2 attempts"))
	})

	ftt.Run("OK", t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(ctx, time.Date(2025, time.January, 1, 2, 3, 4, 0, time.UTC))
		ctx = MockRequestState(ctx, state)
		ctx = cryptorand.MockForTest(ctx, 0)

		req := simpliestValidRequest("visible-pool")
		req.PoolTaskTemplate = apipb.NewTaskRequest_SKIP

		_, err := srv.NewTask(ctx, req)
		assert.NoErr(t, err)
	})

	ftt.Run("Fail", t, func(t *ftt.Test) {
		ctx = MockRequestState(ctx, state)

		// TaskRequestID exists without TaskResultSummary.
		id := "65aba3a3e6b00000"
		tri := &model.TaskRequestID{
			Key:    model.TaskRequestIDKey(ctx, "user:test@example.com:request-id"),
			TaskID: id,
		}
		assert.NoErr(t, datastore.Put(ctx, tri))

		req := simpliestValidRequest("visible-pool")
		req.PoolTaskTemplate = apipb.NewTaskRequest_SKIP
		req.RequestUuid = "request-id"

		res, err := srv.NewTask(ctx, req)
		assert.That(t, err, grpccode.ShouldBe(codes.Internal))
		assert.Loosely(t, res, should.BeNil)
	})
}

func TestApplyRBE(t *testing.T) {
	t.Parallel()

	ftt.Run("applyRBE", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(123)))
		state := NewMockedRequestState()
		poolCfg := state.Configs.MockPool("pool", "project:pool-realm")

		setRBEInConfig := func(instance string, taskPercent int) context.Context {
			if instance != "" {
				poolCfg.RbeMigration = &configpb.Pool_RBEMigration{
					RbeInstance:    instance,
					RbeModePercent: int32(taskPercent),
					BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
						{Mode: configpb.Pool_RBEMigration_BotModeAllocation_HYBRID, Percent: 100},
					},
				}
			}
			return MockRequestState(ctx, state)
		}

		request := func(extraTag string) *apipb.NewTaskRequest {
			req := simpliestValidRequest("pool")
			req.PoolTaskTemplate = apipb.NewTaskRequest_SKIP
			if extraTag != "" {
				req.Tags = append(req.Tags, extraTag)
			}
			return req
		}

		t.Run("no_rbe_in_config", func(t *ftt.Test) {
			ctx := setRBEInConfig("", 0)
			req := request("")
			ents, err := toTaskRequestEntities(ctx, req, "pool")
			assert.NoErr(t, err)
			assert.That(t, ents.request.RBEInstance, should.Equal(""))
			assert.That(t, ents.request.Tags, should.NotContain("rbe:none"))
		})

		t.Run("prevent_rbe_from_request", func(t *ftt.Test) {
			ctx := setRBEInConfig("rbe_instance", 50)
			req := request("rbe:prevent")
			ents, err := toTaskRequestEntities(ctx, req, "pool")
			assert.NoErr(t, err)
			assert.That(t, ents.request.RBEInstance, should.Equal(""))
			assert.That(t, ents.request.Tags, should.Contain("rbe:none"))
		})

		t.Run("allow_rbe_from_request", func(t *ftt.Test) {
			ctx := setRBEInConfig("rbe_instance", 50)
			req := request("rbe:allow")
			ents, err := toTaskRequestEntities(ctx, req, "pool")
			assert.NoErr(t, err)
			assert.That(t, ents.request.RBEInstance, should.Equal("rbe_instance"))
			assert.That(t, ents.request.Tags, should.Contain("rbe:rbe_instance"))
		})

		t.Run("rbe_0", func(t *ftt.Test) {
			ctx := setRBEInConfig("rbe_instance", 0)
			req := request("")
			ents, err := toTaskRequestEntities(ctx, req, "pool")
			assert.NoErr(t, err)
			assert.That(t, ents.request.RBEInstance, should.Equal(""))
			assert.That(t, ents.request.Tags, should.Contain("rbe:none"))
		})

		t.Run("rbe_100", func(t *ftt.Test) {
			ctx := setRBEInConfig("rbe_instance", 100)
			req := request("")
			ents, err := toTaskRequestEntities(ctx, req, "pool")
			assert.NoErr(t, err)
			assert.That(t, ents.request.RBEInstance, should.Equal("rbe_instance"))
			assert.That(t, ents.request.Tags, should.Contain("rbe:rbe_instance"))
		})

		t.Run("random_dice", func(t *ftt.Test) {
			ctx := setRBEInConfig("rbe_instance", 50)
			req := request("")
			ents, err := toTaskRequestEntities(ctx, req, "pool")
			assert.NoErr(t, err)
			assert.That(t, ents.request.RBEInstance, should.Equal("rbe_instance"))
			assert.That(t, ents.request.Tags, should.Contain("rbe:rbe_instance"))
		})
	})
}
