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
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey("Update Service", t, func() {
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
			Id:              serviceName,
			ServiceEndpoint: ts.Host,
		}
		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
			common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
				Services: []*cfgcommonpb.Service{
					serviceInfo,
				},
			},
		})

		Convey("First time update", func() {
			service := &model.Service{
				Name: serviceName,
			}
			So(datastore.Get(ctx, service), ShouldErrLike, datastore.ErrNoSuchEntity)
			So(Update(ctx), ShouldBeNil)
			So(datastore.Get(ctx, service), ShouldBeNil)
			So(service.Info, ShouldResembleProto, serviceInfo)
			So(service.Metadata, ShouldResembleProto, serviceMetadata)
			So(service.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
		})

		Convey("Update existing", func() {
			So(Update(ctx), ShouldBeNil)
			Convey("Info changed", func() {
				updatedInfo := proto.Clone(serviceInfo).(*cfgcommonpb.Service)
				updatedInfo.Owners = append(updatedInfo.Owners, "new-owner@example.com")
				testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
					common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
						Services: []*cfgcommonpb.Service{
							updatedInfo,
						},
					},
				})
				So(Update(ctx), ShouldBeNil)
				service := &model.Service{
					Name: serviceName,
				}
				So(datastore.Get(ctx, service), ShouldBeNil)
				So(service.Info, ShouldResembleProto, updatedInfo)
				So(service.Metadata, ShouldResembleProto, serviceMetadata)
				So(service.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
			})
			Convey("Metadata changed", func() {
				updated := &cfgcommonpb.ServiceMetadata{
					ConfigPatterns: []*cfgcommonpb.ConfigPattern{
						{ConfigSet: string(config.MustProjectSet("foo")), Path: "exact:bar.cfg"},
						{ConfigSet: string(config.MustProjectSet("foo")), Path: "exact:baz.cfg"},
					},
				}
				srv.sm = updated
				So(Update(ctx), ShouldBeNil)
				service := &model.Service{
					Name: serviceName,
				}
				So(datastore.Get(ctx, service), ShouldBeNil)
				So(service.Info, ShouldResembleProto, serviceInfo)
				So(service.Metadata, ShouldResembleProto, updated)
				So(service.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
			})
		})

		Convey("Error for invalid metadata", func() {
			srv.sm = &cfgcommonpb.ServiceMetadata{
				ConfigPatterns: []*cfgcommonpb.ConfigPattern{
					{ConfigSet: string(config.MustProjectSet("foo")), Path: "regex:["},
				},
			}
			So(Update(ctx), ShouldErrLike, "invalid metadata for service")
		})

		Convey("Skip update if nothing changed", func() {
			So(Update(ctx), ShouldBeNil)
			service := &model.Service{
				Name: serviceName,
			}
			So(datastore.Get(ctx, service), ShouldBeNil)
			prevUpdateTime := service.UpdateTime
			tc := clock.Get(ctx).(testclock.TestClock)
			tc.Add(1 * time.Hour)
			So(Update(ctx), ShouldBeNil)
			service = &model.Service{
				Name: serviceName,
			}
			So(datastore.Get(ctx, service), ShouldBeNil)
			So(service.UpdateTime, ShouldEqual, prevUpdateTime)
		})

		Convey("Update Service without metadata", func() {
			testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
				common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
					Services: []*cfgcommonpb.Service{
						{
							Id: serviceName,
						},
					},
				},
			})
			So(Update(ctx), ShouldBeNil)
			service := &model.Service{
				Name: serviceName,
			}
			So(datastore.Get(ctx, service), ShouldBeNil)
			So(service.Info, ShouldResembleProto, &cfgcommonpb.Service{
				Id: serviceName,
			})
			So(service.Metadata, ShouldBeNil)
			So(service.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
			Convey("Update again", func() {
				testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
					common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
						Services: []*cfgcommonpb.Service{
							{
								Id:     serviceName,
								Owners: []string{"owner@example.com"},
							},
						},
					},
				})
				So(Update(ctx), ShouldBeNil)
				service := &model.Service{
					Name: serviceName,
				}
				So(datastore.Get(ctx, service), ShouldBeNil)
				So(service.Info, ShouldResembleProto, &cfgcommonpb.Service{
					Id:     serviceName,
					Owners: []string{"owner@example.com"},
				})
			})
		})

		Convey("Update self", func() {
			serviceInfo := &cfgcommonpb.Service{
				Id:              testutil.AppID,
				ServiceEndpoint: ts.Host,
				// Add the service endpoint to ensure LUCI Config update its own
				// Service entity without making rpc call to itself.
			}
			testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
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
			So(Update(ctx), ShouldBeNil)
			So(datastore.Get(ctx, service), ShouldBeNil)
			So(service.Info, ShouldResembleProto, serviceInfo)
			So(service.Metadata, ShouldResembleProto, &cfgcommonpb.ServiceMetadata{
				ConfigPatterns: []*cfgcommonpb.ConfigPattern{
					{
						ConfigSet: "exact:services/" + testutil.AppID,
						Path:      "exact:foo.cfg",
					},
				},
			})
			So(service.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
		})

		Convey("Delete Service entity for deleted service", func() {
			So(Update(ctx), ShouldBeNil)
			er, err := datastore.Exists(ctx, datastore.MakeKey(ctx, model.ServiceKind, serviceName))
			So(err, ShouldBeNil)
			So(er.All(), ShouldBeTrue)
			testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
				common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
					Services: []*cfgcommonpb.Service{}, // delete the existing Service
				},
			})
			So(Update(ctx), ShouldBeNil)
			er, err = datastore.Exists(ctx, datastore.MakeKey(ctx, model.ServiceKind, serviceName))
			So(err, ShouldBeNil)
			So(er.Any(), ShouldBeFalse)
		})

		Convey("Legacy Metadata", func() {
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
			testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
				common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
					Services: []*cfgcommonpb.Service{
						serviceInfo,
					},
				},
			})

			Convey("First time update", func() {
				service := &model.Service{
					Name: serviceName,
				}
				So(datastore.Get(ctx, service), ShouldErrLike, datastore.ErrNoSuchEntity)
				So(Update(ctx), ShouldBeNil)
				So(datastore.Get(ctx, service), ShouldBeNil)
				So(service.Info, ShouldResembleProto, serviceInfo)
				So(service.LegacyMetadata, ShouldResembleProto, legacyMetadata)
				So(service.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
			})

			Convey("Update existing", func() {
				So(Update(ctx), ShouldBeNil)
				Convey("Service info changed", func() {
					updatedInfo := proto.Clone(serviceInfo).(*cfgcommonpb.Service)
					updatedInfo.Owners = append(updatedInfo.Owners, "new-owner@example.com")
					testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
						common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
							Services: []*cfgcommonpb.Service{
								updatedInfo,
							},
						},
					})
					So(Update(ctx), ShouldBeNil)
					service := &model.Service{
						Name: serviceName,
					}
					So(datastore.Get(ctx, service), ShouldBeNil)
					So(service.Info, ShouldResembleProto, updatedInfo)
					So(service.LegacyMetadata, ShouldResembleProto, legacyMetadata)
					So(service.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
				})
				Convey("Legacy metadata changed", func() {
					legacyMetadata = proto.Clone(legacyMetadata).(*cfgcommonpb.ServiceDynamicMetadata)
					legacyMetadata.SupportsGzipCompression = false
					So(Update(ctx), ShouldBeNil)
					service := &model.Service{
						Name: serviceName,
					}
					So(datastore.Get(ctx, service), ShouldBeNil)
					So(service.Info, ShouldResembleProto, serviceInfo)
					So(service.LegacyMetadata, ShouldResembleProto, legacyMetadata)
					So(service.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
				})
			})

			Convey("Error for invalid legacy metadata", func() {
				Convey("Invalid regex", func() {
					legacyMetadata = proto.Clone(legacyMetadata).(*cfgcommonpb.ServiceDynamicMetadata)
					legacyMetadata.Validation.Patterns = []*cfgcommonpb.ConfigPattern{
						{ConfigSet: string(config.MustProjectSet("foo")), Path: "regex:["},
					}
				})
				Convey("Empty url", func() {
					legacyMetadata = proto.Clone(legacyMetadata).(*cfgcommonpb.ServiceDynamicMetadata)
					legacyMetadata.Validation.Url = ""
				})
				Convey("Invalid url", func() {
					legacyMetadata = proto.Clone(legacyMetadata).(*cfgcommonpb.ServiceDynamicMetadata)
					legacyMetadata.Validation.Url = "http://example.com\\validate"
				})
				So(Update(ctx), ShouldErrLike, "invalid legacy metadata for service")
			})

			Convey("Upgrade from legacy to new", func() {
				So(Update(ctx), ShouldBeNil)
				service := &model.Service{
					Name: serviceName,
				}
				So(datastore.Get(ctx, service), ShouldBeNil)
				So(service.Info, ShouldResembleProto, serviceInfo)
				So(service.Metadata, ShouldBeNil)
				So(service.LegacyMetadata, ShouldNotBeNil)

				newInfo := proto.Clone(serviceInfo).(*cfgcommonpb.Service)
				newInfo.ServiceEndpoint = ts.Host
				newInfo.MetadataUrl = ""
				testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
					common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
						Services: []*cfgcommonpb.Service{
							newInfo,
						},
					},
				})
				So(Update(ctx), ShouldBeNil)
				service = &model.Service{
					Name: serviceName,
				}
				So(datastore.Get(ctx, service), ShouldBeNil)
				So(service.Info, ShouldResembleProto, newInfo)
				So(service.Metadata, ShouldNotBeNil)
				So(service.LegacyMetadata, ShouldBeNil)
			})
		})
	})
}
