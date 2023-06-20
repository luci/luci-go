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

func TestUpdateMetadata(t *testing.T) {
	t.Parallel()

	Convey("Update Metadata", t, func() {
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

		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
			common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
				Services: []*cfgcommonpb.Service{
					{
						Id:              serviceName,
						ServiceEndpoint: ts.Host,
					},
				},
			},
		})

		Convey("First time update", func() {
			metadataEntity := &model.ServiceMetadata{
				ServiceName: serviceName,
			}
			So(datastore.Get(ctx, metadataEntity), ShouldErrLike, datastore.ErrNoSuchEntity)
			So(UpdateMetadata(ctx), ShouldBeNil)
			So(datastore.Get(ctx, metadataEntity), ShouldBeNil)
			So(metadataEntity.Metadata, ShouldResembleProto, serviceMetadata)
			So(metadataEntity.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
		})

		Convey("Update existing", func() {
			So(UpdateMetadata(ctx), ShouldBeNil)
			updated := &cfgcommonpb.ServiceMetadata{
				ConfigPatterns: []*cfgcommonpb.ConfigPattern{
					{ConfigSet: string(config.MustProjectSet("foo")), Path: "exact:bar.cfg"},
					{ConfigSet: string(config.MustProjectSet("foo")), Path: "exact:baz.cfg"},
				},
			}
			srv.sm = updated
			So(UpdateMetadata(ctx), ShouldBeNil)
			metadataEntity := &model.ServiceMetadata{
				ServiceName: serviceName,
			}
			So(datastore.Get(ctx, metadataEntity), ShouldBeNil)
			So(metadataEntity.Metadata, ShouldResembleProto, updated)
			So(metadataEntity.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
		})

		Convey("Skip update for same metadata", func() {
			So(UpdateMetadata(ctx), ShouldBeNil)
			metadataEntity := &model.ServiceMetadata{
				ServiceName: serviceName,
			}
			So(datastore.Get(ctx, metadataEntity), ShouldBeNil)
			prevUpdateTime := metadataEntity.UpdateTime
			tc := clock.Get(ctx).(testclock.TestClock)
			tc.Add(1 * time.Hour)
			So(UpdateMetadata(ctx), ShouldBeNil)
			metadataEntity = &model.ServiceMetadata{
				ServiceName: serviceName,
			}
			So(datastore.Get(ctx, metadataEntity), ShouldBeNil)
			So(metadataEntity.UpdateTime, ShouldEqual, prevUpdateTime)
		})

		Convey("Skip update for service without endpoint", func() {
			testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
				common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
					Services: []*cfgcommonpb.Service{
						{
							Id: serviceName,
						},
					},
				},
			})
			So(UpdateMetadata(ctx), ShouldBeNil)
			metadataEntity := &model.ServiceMetadata{
				ServiceName: serviceName,
			}
			So(datastore.Get(ctx, metadataEntity), ShouldErrLike, datastore.ErrNoSuchEntity)
		})

		Convey("Legacy Metadata", func() {
			legacyMetadata := &cfgcommonpb.ServiceDynamicMetadata{
				Version: "1.0",
				Validation: &cfgcommonpb.Validator{
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

			testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
				common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
					Services: []*cfgcommonpb.Service{
						{
							Id:          serviceName,
							MetadataUrl: legacyTestSrv.URL,
						},
					},
				},
			})

			Convey("First time update", func() {
				metadataEntity := &model.ServiceMetadata{
					ServiceName: serviceName,
				}
				So(datastore.Get(ctx, metadataEntity), ShouldErrLike, datastore.ErrNoSuchEntity)
				So(UpdateMetadata(ctx), ShouldBeNil)
				So(datastore.Get(ctx, metadataEntity), ShouldBeNil)
				So(metadataEntity.LegacyMetadata, ShouldResembleProto, legacyMetadata)
				So(metadataEntity.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
			})

			Convey("Update existing", func() {
				So(UpdateMetadata(ctx), ShouldBeNil)
				legacyMetadata = proto.Clone(legacyMetadata).(*cfgcommonpb.ServiceDynamicMetadata)
				legacyMetadata.SupportsGzipCompression = false
				So(UpdateMetadata(ctx), ShouldBeNil)
				metadataEntity := &model.ServiceMetadata{
					ServiceName: serviceName,
				}
				So(datastore.Get(ctx, metadataEntity), ShouldBeNil)
				So(metadataEntity.LegacyMetadata, ShouldResembleProto, legacyMetadata)
				So(metadataEntity.UpdateTime, ShouldEqual, clock.Now(ctx).UTC())
			})

			Convey("Skip update for same metadata", func() {
				So(UpdateMetadata(ctx), ShouldBeNil)
				metadataEntity := &model.ServiceMetadata{
					ServiceName: serviceName,
				}
				So(datastore.Get(ctx, metadataEntity), ShouldBeNil)
				prevUpdateTime := metadataEntity.UpdateTime
				tc := clock.Get(ctx).(testclock.TestClock)
				tc.Add(1 * time.Hour)
				So(UpdateMetadata(ctx), ShouldBeNil)
				metadataEntity = &model.ServiceMetadata{
					ServiceName: serviceName,
				}
				So(datastore.Get(ctx, metadataEntity), ShouldBeNil)
				So(metadataEntity.UpdateTime, ShouldEqual, prevUpdateTime)
			})

			Convey("Upgrade from legacy to new", func() {
				So(UpdateMetadata(ctx), ShouldBeNil)
				metadataEntity := &model.ServiceMetadata{
					ServiceName: serviceName,
				}
				So(datastore.Get(ctx, metadataEntity), ShouldBeNil)
				So(metadataEntity.Metadata, ShouldBeNil)
				So(metadataEntity.LegacyMetadata, ShouldNotBeNil)

				testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
					common.ServiceRegistryFilePath: &cfgcommonpb.ServicesCfg{
						Services: []*cfgcommonpb.Service{
							{
								Id:              serviceName,
								ServiceEndpoint: ts.Host,
							},
						},
					},
				})
				So(UpdateMetadata(ctx), ShouldBeNil)
				metadataEntity = &model.ServiceMetadata{
					ServiceName: serviceName,
				}
				So(datastore.Get(ctx, metadataEntity), ShouldBeNil)
				So(metadataEntity.Metadata, ShouldNotBeNil)
				So(metadataEntity.LegacyMetadata, ShouldBeNil)
			})
		})
	})
}
