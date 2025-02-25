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

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"
)

type testConsumerServer struct {
	cfgcommonpb.UnimplementedConsumerServer
	sm *cfgcommonpb.ServiceMetadata
}

func (srv *testConsumerServer) GetMetadata(context.Context, *emptypb.Empty) (*cfgcommonpb.ServiceMetadata, error) {
	return srv.sm, nil
}

func TestUpdateService(t *testing.T) {
	t.Parallel()

	ftt.Run("Update Service", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		ctx = authtest.MockAuthConfig(ctx)

		ts := &prpctest.Server{}
		srv := &testConsumerServer{}
		cfgcommonpb.RegisterConsumerServer(ts, srv)
		ts.Start(ctx)
		defer ts.Close()

		const serviceName = "my-service"
		serviceMetadata := &cfgcommonpb.ServiceMetadata{
			ConfigPatterns: []*cfgcommonpb.ConfigPattern{
				{ConfigSet: string(config.MustProjectSet("foo")), Path: "exact:bar.cfg"},
			},
		}
		srv.sm = serviceMetadata
		serviceInfo := &cfgcommonpb.Service{
			Id:       serviceName,
			Hostname: ts.Host,
		}
		testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
			common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
				Services: []*cfgcommonpb.Service{
					serviceInfo,
				},
			},
		})

		t.Run("First time update", func(t *ftt.Test) {
			service := &model.Service{
				Name: serviceName,
			}
			assert.Loosely(t, datastore.Get(ctx, service), should.ErrLike(datastore.ErrNoSuchEntity))
			assert.Loosely(t, Update(ctx), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
			assert.Loosely(t, service.Info, should.Match(serviceInfo))
			assert.Loosely(t, service.Metadata, should.Match(serviceMetadata))
			assert.Loosely(t, service.UpdateTime, should.Match(clock.Now(ctx).UTC()))
		})

		t.Run("Update existing", func(t *ftt.Test) {
			assert.Loosely(t, Update(ctx), should.BeNil)
			t.Run("Info changed", func(t *ftt.Test) {
				updatedInfo := proto.Clone(serviceInfo).(*cfgcommonpb.Service)
				updatedInfo.Owners = append(updatedInfo.Owners, "new-owner@example.com")
				testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
					common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
						Services: []*cfgcommonpb.Service{
							updatedInfo,
						},
					},
				})
				assert.Loosely(t, Update(ctx), should.BeNil)
				service := &model.Service{
					Name: serviceName,
				}
				assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
				assert.Loosely(t, service.Info, should.Match(updatedInfo))
				assert.Loosely(t, service.Metadata, should.Match(serviceMetadata))
				assert.Loosely(t, service.UpdateTime, should.Match(clock.Now(ctx).UTC()))
			})
			t.Run("Metadata changed", func(t *ftt.Test) {
				updated := &cfgcommonpb.ServiceMetadata{
					ConfigPatterns: []*cfgcommonpb.ConfigPattern{
						{ConfigSet: string(config.MustProjectSet("foo")), Path: "exact:bar.cfg"},
						{ConfigSet: string(config.MustProjectSet("foo")), Path: "exact:baz.cfg"},
					},
				}
				srv.sm = updated
				assert.Loosely(t, Update(ctx), should.BeNil)
				service := &model.Service{
					Name: serviceName,
				}
				assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
				assert.Loosely(t, service.Info, should.Match(serviceInfo))
				assert.Loosely(t, service.Metadata, should.Match(updated))
				assert.Loosely(t, service.UpdateTime, should.Match(clock.Now(ctx).UTC()))
			})
		})

		t.Run("Error for invalid metadata", func(t *ftt.Test) {
			srv.sm = &cfgcommonpb.ServiceMetadata{
				ConfigPatterns: []*cfgcommonpb.ConfigPattern{
					{ConfigSet: string(config.MustProjectSet("foo")), Path: "regex:["},
				},
			}
			assert.Loosely(t, Update(ctx), should.ErrLike("invalid metadata for service"))
		})

		t.Run("Skip update if nothing changed", func(t *ftt.Test) {
			assert.Loosely(t, Update(ctx), should.BeNil)
			service := &model.Service{
				Name: serviceName,
			}
			assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
			prevUpdateTime := service.UpdateTime
			tc := clock.Get(ctx).(testclock.TestClock)
			tc.Add(1 * time.Hour)
			assert.Loosely(t, Update(ctx), should.BeNil)
			service = &model.Service{
				Name: serviceName,
			}
			assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
			assert.Loosely(t, service.UpdateTime, should.Match(prevUpdateTime))
		})

		t.Run("Update Service without metadata", func(t *ftt.Test) {
			testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
				common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
					Services: []*cfgcommonpb.Service{
						{
							Id: serviceName,
						},
					},
				},
			})
			assert.Loosely(t, Update(ctx), should.BeNil)
			service := &model.Service{
				Name: serviceName,
			}
			assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
			assert.Loosely(t, service.Info, should.Match(&cfgcommonpb.Service{
				Id: serviceName,
			}))
			assert.Loosely(t, service.Metadata, should.BeNil)
			assert.Loosely(t, service.UpdateTime, should.Match(clock.Now(ctx).UTC()))
			t.Run("Update again", func(t *ftt.Test) {
				testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
					common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
						Services: []*cfgcommonpb.Service{
							{
								Id:     serviceName,
								Owners: []string{"owner@example.com"},
							},
						},
					},
				})
				assert.Loosely(t, Update(ctx), should.BeNil)
				service := &model.Service{
					Name: serviceName,
				}
				assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
				assert.Loosely(t, service.Info, should.Match(&cfgcommonpb.Service{
					Id:     serviceName,
					Owners: []string{"owner@example.com"},
				}))
			})
		})

		t.Run("Update self", func(t *ftt.Test) {
			serviceInfo := &cfgcommonpb.Service{
				Id:       testutil.AppID,
				Hostname: ts.Host,
				// Add the service endpoint to ensure LUCI Config update its own
				// Service entity without making rpc call to itself.
			}
			testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
				common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
					Services: []*cfgcommonpb.Service{
						serviceInfo,
					},
				},
			})
			validation.Rules.Add("exact:services/"+testutil.AppID, "exact:foo.cfg", func(ctx *validation.Context, configSet, path string, content []byte) error { return nil })
			service := &model.Service{
				Name: testutil.AppID,
			}
			assert.Loosely(t, Update(ctx), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
			assert.Loosely(t, service.Info, should.Match(serviceInfo))
			assert.Loosely(t, service.Metadata, should.Match(&cfgcommonpb.ServiceMetadata{
				ConfigPatterns: []*cfgcommonpb.ConfigPattern{
					{
						ConfigSet: "exact:services/" + testutil.AppID,
						Path:      "exact:foo.cfg",
					},
				},
			}))
			assert.Loosely(t, service.UpdateTime, should.Match(clock.Now(ctx).UTC()))
		})

		t.Run("Delete Service entity for deleted service", func(t *ftt.Test) {
			assert.Loosely(t, Update(ctx), should.BeNil)
			er, err := datastore.Exists(ctx, datastore.MakeKey(ctx, model.ServiceKind, serviceName))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, er.All(), should.BeTrue)
			testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
				common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
					Services: []*cfgcommonpb.Service{}, // delete the existing Service
				},
			})
			assert.Loosely(t, Update(ctx), should.BeNil)
			er, err = datastore.Exists(ctx, datastore.MakeKey(ctx, model.ServiceKind, serviceName))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, er.Any(), should.BeFalse)
		})

		t.Run("Legacy Metadata", func(t *ftt.Test) {
			legacyMetadata := &cfgcommonpb.ServiceDynamicMetadata{
				Version: "1.0",
				Validation: &cfgcommonpb.Validator{
					Url: "https://example.com/validate",
					Patterns: []*cfgcommonpb.ConfigPattern{
						{ConfigSet: string(config.MustProjectSet("foo")), Path: "exact:bar.cfg"},
					},
				},
				SupportsGzipCompression: true,
			}
			var legacySrvErrMsg string

			legacyTestSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if legacySrvErrMsg != "" {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprint(w, legacySrvErrMsg)
					return
				}

				switch bytes, err := protojson.Marshal(legacyMetadata); {
				case err != nil:
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintf(w, "%s", err)
				default:
					w.Write(bytes)
				}
			}))
			defer legacyTestSrv.Close()

			serviceInfo := &cfgcommonpb.Service{
				Id:          serviceName,
				MetadataUrl: legacyTestSrv.URL,
			}
			testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
				common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
					Services: []*cfgcommonpb.Service{
						serviceInfo,
					},
				},
			})

			t.Run("First time update", func(t *ftt.Test) {
				service := &model.Service{
					Name: serviceName,
				}
				assert.Loosely(t, datastore.Get(ctx, service), should.ErrLike(datastore.ErrNoSuchEntity))
				assert.Loosely(t, Update(ctx), should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
				assert.Loosely(t, service.Info, should.Match(serviceInfo))
				assert.Loosely(t, service.LegacyMetadata, should.Match(legacyMetadata))
				assert.Loosely(t, service.UpdateTime, should.Match(clock.Now(ctx).UTC()))
			})

			t.Run("Update existing", func(t *ftt.Test) {
				assert.Loosely(t, Update(ctx), should.BeNil)
				t.Run("Service info changed", func(t *ftt.Test) {
					updatedInfo := proto.Clone(serviceInfo).(*cfgcommonpb.Service)
					updatedInfo.Owners = append(updatedInfo.Owners, "new-owner@example.com")
					testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
						common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
							Services: []*cfgcommonpb.Service{
								updatedInfo,
							},
						},
					})
					assert.Loosely(t, Update(ctx), should.BeNil)
					service := &model.Service{
						Name: serviceName,
					}
					assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
					assert.Loosely(t, service.Info, should.Match(updatedInfo))
					assert.Loosely(t, service.LegacyMetadata, should.Match(legacyMetadata))
					assert.Loosely(t, service.UpdateTime, should.Match(clock.Now(ctx).UTC()))
				})
				t.Run("Legacy metadata changed", func(t *ftt.Test) {
					legacyMetadata = proto.Clone(legacyMetadata).(*cfgcommonpb.ServiceDynamicMetadata)
					legacyMetadata.SupportsGzipCompression = false
					assert.Loosely(t, Update(ctx), should.BeNil)
					service := &model.Service{
						Name: serviceName,
					}
					assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
					assert.Loosely(t, service.Info, should.Match(serviceInfo))
					assert.Loosely(t, service.LegacyMetadata, should.Match(legacyMetadata))
					assert.Loosely(t, service.UpdateTime, should.Match(clock.Now(ctx).UTC()))
				})
			})

			t.Run("Error for invalid legacy metadata", func(t *ftt.Test) {
				check := func(t testing.TB) {
					t.Helper()
					assert.Loosely(t, Update(ctx), should.ErrLike("invalid legacy metadata for service"), truth.LineContext())
				}
				t.Run("Invalid regex", func(t *ftt.Test) {
					legacyMetadata = proto.Clone(legacyMetadata).(*cfgcommonpb.ServiceDynamicMetadata)
					legacyMetadata.Validation.Patterns = []*cfgcommonpb.ConfigPattern{
						{ConfigSet: string(config.MustProjectSet("foo")), Path: "regex:["},
					}
					check(t)
				})
				t.Run("Empty url", func(t *ftt.Test) {
					legacyMetadata = proto.Clone(legacyMetadata).(*cfgcommonpb.ServiceDynamicMetadata)
					legacyMetadata.Validation.Url = ""
					check(t)
				})
				t.Run("Invalid url", func(t *ftt.Test) {
					legacyMetadata = proto.Clone(legacyMetadata).(*cfgcommonpb.ServiceDynamicMetadata)
					legacyMetadata.Validation.Url = "http://example.com\\validate"
					check(t)
				})
			})

			t.Run("Upgrade from legacy to new", func(t *ftt.Test) {
				assert.Loosely(t, Update(ctx), should.BeNil)
				service := &model.Service{
					Name: serviceName,
				}
				assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
				assert.Loosely(t, service.Info, should.Match(serviceInfo))
				assert.Loosely(t, service.Metadata, should.BeNil)
				assert.Loosely(t, service.LegacyMetadata, should.NotBeNil)

				newInfo := proto.Clone(serviceInfo).(*cfgcommonpb.Service)
				newInfo.Hostname = ts.Host
				newInfo.MetadataUrl = ""
				testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
					common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
						Services: []*cfgcommonpb.Service{
							newInfo,
						},
					},
				})
				assert.Loosely(t, Update(ctx), should.BeNil)
				service = &model.Service{
					Name: serviceName,
				}
				assert.Loosely(t, datastore.Get(ctx, service), should.BeNil)
				assert.Loosely(t, service.Info, should.Match(newInfo))
				assert.Loosely(t, service.Metadata, should.NotBeNil)
				assert.Loosely(t, service.LegacyMetadata, should.BeNil)
			})
		})
	})
}
