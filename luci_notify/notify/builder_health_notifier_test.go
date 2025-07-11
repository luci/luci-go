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

package notify

import (
	"bytes"
	"compress/gzip"
	"context"
	"io/ioutil"
	"testing"

	"github.com/golang/mock/gomock"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
)

func TestNotifyOwnersHelper(t *testing.T) {
	ftt.Run("Test environment", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		c = common.SetAppIDForTest(c, "luci-notify-test")

		datastore.GetTestable(c).Consistent(true)
		c = memlogger.Use(c)
		mockBuildersClient := buildbucketpb.NewMockBuildersClient(gomock.NewController(t))

		t.Run("Success", func(t *ftt.Test) {
			mockBHN := []*notifypb.BuilderHealthNotifier{
				&notifypb.BuilderHealthNotifier{
					OwnerEmail: "test@google.com",
					Builders: []*notifypb.Builder{
						&notifypb.Builder{
							Bucket: "ci",
							Name:   "builder1",
						},
						&notifypb.Builder{
							Bucket: "ci",
							Name:   "builder2",
						},
						&notifypb.Builder{
							Bucket: "ci",
							Name:   "builder3",
						},
						&notifypb.Builder{
							Bucket: "ci",
							Name:   "builder4",
						},
					},
				},
			}
			expectedReq1 := &buildbucketpb.GetBuilderRequest{
				Id: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "builder2",
				},
				Mask: &buildbucketpb.BuilderMask{
					Type: 2,
				},
			}
			mockBuildersClient.
				EXPECT().
				GetBuilder(gomock.Any(), proto.MatcherEqual(expectedReq1)).
				MaxTimes(1).
				Return(&buildbucketpb.BuilderItem{
					Metadata: &buildbucketpb.BuilderMetadata{
						Health: &buildbucketpb.HealthStatus{
							HealthScore: 0,
							Description: "Your builder is not healthy with health score of 0",
						},
					},
				}, nil)
			expectedReq2 := &buildbucketpb.GetBuilderRequest{
				Id: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "builder3",
				},
				Mask: &buildbucketpb.BuilderMask{
					Type: 2,
				},
			}
			mockBuildersClient.
				EXPECT().
				GetBuilder(gomock.Any(), proto.MatcherEqual(expectedReq2)).
				MaxTimes(1).
				Return(&buildbucketpb.BuilderItem{
					Metadata: &buildbucketpb.BuilderMetadata{
						Health: &buildbucketpb.HealthStatus{
							HealthScore: 5,
							Description: "Your builder is not healthy",
						},
					},
				}, nil)
			expectedReq3 := &buildbucketpb.GetBuilderRequest{
				Id: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "builder1",
				},
				Mask: &buildbucketpb.BuilderMask{
					Type: 2,
				},
			}
			mockBuildersClient.
				EXPECT().
				GetBuilder(gomock.Any(), proto.MatcherEqual(expectedReq3)).
				MaxTimes(1).
				Return(&buildbucketpb.BuilderItem{
					Metadata: &buildbucketpb.BuilderMetadata{
						Health: &buildbucketpb.HealthStatus{
							HealthScore: 10,
							Description: "Your builder is healthy",
						},
					},
				}, nil)
			expectedReq4 := &buildbucketpb.GetBuilderRequest{
				Id: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "builder4",
				},
				Mask: &buildbucketpb.BuilderMask{
					Type: 2,
				},
			}
			mockBuildersClient.
				EXPECT().
				GetBuilder(gomock.Any(), proto.MatcherEqual(expectedReq4)).
				MaxTimes(1).
				Return(&buildbucketpb.BuilderItem{
					Metadata: &buildbucketpb.BuilderMetadata{
						Health: &buildbucketpb.HealthStatus{
							HealthScore: 10,
							Description: "Your builder is healthy",
						},
					},
				}, nil)

			tasks, err := getNotifyOwnersTasks(c, mockBHN, mockBuildersClient, "chromium")
			assert.Loosely(t, err, should.BeNil)
			task := tasks["test@google.com"]
			assert.Loosely(t, task.Recipients, should.Match([]string{"test@google.com"}))
			assert.Loosely(t, task.Subject, should.Equal("Builder Health For test@google.com - 2 of 4 Are in Bad Health"))
			expectedBody :=`
	<html>
	<head>
		<meta charset="utf-8">
	</head>
	<body>
		<p>Hello,</p>
		<p>You are receiving this because <strong>test@google.com</strong> is subscribed to builder health notifier. <strong>2</strong> of your <strong>4</strong> builders are in bad health.</p>

		<p><strong>Unhealthy Builders:</strong></p><ul><li><strong><a href="https://ci.chromium.org/ui/p/chromium/builders/ci/builder2">chromium.ci:builder2</a></strong><p style="margin-left:30px;">Your builder is not healthy with health score of 0</p></li><li><strong><a href="https://ci.chromium.org/ui/p/chromium/builders/ci/builder3">chromium.ci:builder3</a></strong><p style="margin-left:30px;">Your builder is not healthy</p></li></ul><p><strong>Healthy Builders:</strong></p><ul><li><strong><a href="https://ci.chromium.org/ui/p/chromium/builders/ci/builder1">chromium.ci:builder1</a></strong><p style="margin-left:30px;">Your builder is healthy</p></li><li><strong><a href="https://ci.chromium.org/ui/p/chromium/builders/ci/builder4">chromium.ci:builder4</a></strong><p style="margin-left:30px;">Your builder is healthy</p></li></ul>

		<p>For more information on builder health, please see the <a href="https://chromium.googlesource.com/chromium/src/+/HEAD/docs/infra/builder_health_indicators.md">Builder Health Documentation</a>.</p>
	</body>
	</html>
	`
			reader, _ := gzip.NewReader(bytes.NewReader(task.BodyGzip))
			decompressedBytes, _ := ioutil.ReadAll(reader)
			finalBody := string(decompressedBytes)
			assert.Loosely(t, finalBody, should.Equal(expectedBody))
		})

		t.Run("No metadata or healthstatus", func(t *ftt.Test) {
			mockBHN := []*notifypb.BuilderHealthNotifier{
				&notifypb.BuilderHealthNotifier{
					OwnerEmail: "test@google.com",
					Builders: []*notifypb.Builder{
						&notifypb.Builder{
							Bucket: "ci",
							Name:   "healthyBuilder1",
						},
						&notifypb.Builder{
							Bucket: "ci",
							Name:   "unhealthyBuilder2",
						},
						&notifypb.Builder{
							Bucket: "ci",
							Name:   "missingMetaBuilder3",
						},
						&notifypb.Builder{
							Bucket: "ci",
							Name:   "missingHealthBuilder4",
						},
					},
				},
			}
			expectedReq1 := &buildbucketpb.GetBuilderRequest{
				Id: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "healthyBuilder1",
				},
				Mask: &buildbucketpb.BuilderMask{
					Type: 2,
				},
			}
			mockBuildersClient.
				EXPECT().
				GetBuilder(gomock.Any(), proto.MatcherEqual(expectedReq1)).
				MaxTimes(1).
				Return(&buildbucketpb.BuilderItem{
					Metadata: &buildbucketpb.BuilderMetadata{
						Health: &buildbucketpb.HealthStatus{
							HealthScore: 10,
							Description: "Your builder is healthy",
						},
					},
				}, nil)
			expectedReq2 := &buildbucketpb.GetBuilderRequest{
				Id: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "unhealthyBuilder2",
				},
				Mask: &buildbucketpb.BuilderMask{
					Type: 2,
				},
			}
			mockBuildersClient.
				EXPECT().
				GetBuilder(gomock.Any(), proto.MatcherEqual(expectedReq2)).
				MaxTimes(1).
				Return(&buildbucketpb.BuilderItem{
					Metadata: &buildbucketpb.BuilderMetadata{
						Health: &buildbucketpb.HealthStatus{
							HealthScore: 0,
							Description: "Your builder is not healthy",
						},
					},
				}, nil)
			expectedReq3 := &buildbucketpb.GetBuilderRequest{
				Id: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "missingMetaBuilder3",
				},
				Mask: &buildbucketpb.BuilderMask{
					Type: 2,
				},
			}
			mockBuildersClient.
				EXPECT().
				GetBuilder(gomock.Any(), proto.MatcherEqual(expectedReq3)).
				MaxTimes(1).
				Return(&buildbucketpb.BuilderItem{
					Metadata: &buildbucketpb.BuilderMetadata{
					},
				}, nil)
			expectedReq4 := &buildbucketpb.GetBuilderRequest{
				Id: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "missingHealthBuilder4",
				},
				Mask: &buildbucketpb.BuilderMask{
					Type: 2,
				},
			}
			mockBuildersClient.
				EXPECT().
				GetBuilder(gomock.Any(), proto.MatcherEqual(expectedReq4)).
				MaxTimes(1).
				Return(&buildbucketpb.BuilderItem{

				}, nil)
			tasks, err := getNotifyOwnersTasks(c, mockBHN, mockBuildersClient, "chromium")
			assert.Loosely(t, err, should.BeNil)
			task := tasks["test@google.com"]
			assert.Loosely(t, task.Recipients, should.Match([]string{"test@google.com"}))
			assert.Loosely(t, task.Subject, should.Equal("Builder Health For test@google.com - 1 of 4 Are in Bad Health"))
			expectedBody := `
	<html>
	<head>
		<meta charset="utf-8">
	</head>
	<body>
		<p>Hello,</p>
		<p>You are receiving this because <strong>test@google.com</strong> is subscribed to builder health notifier. <strong>1</strong> of your <strong>4</strong> builders are in bad health.</p>

		<p><strong>Unhealthy Builders:</strong></p><ul><li><strong><a href="https://ci.chromium.org/ui/p/chromium/builders/ci/unhealthyBuilder2">chromium.ci:unhealthyBuilder2</a></strong><p style="margin-left:30px;">Your builder is not healthy</p></li></ul><p><strong>Healthy Builders:</strong></p><ul><li><strong><a href="https://ci.chromium.org/ui/p/chromium/builders/ci/healthyBuilder1">chromium.ci:healthyBuilder1</a></strong><p style="margin-left:30px;">Your builder is healthy</p></li></ul><p><strong>Unknown Health Builders:</strong></p><ul><li><strong><a href="https://ci.chromium.org/ui/p/chromium/builders/ci/missingMetaBuilder3">chromium.ci:missingMetaBuilder3</a></strong></li><li><strong><a href="https://ci.chromium.org/ui/p/chromium/builders/ci/missingHealthBuilder4">chromium.ci:missingHealthBuilder4</a></strong></li></ul>

		<p>For more information on builder health, please see the <a href="https://chromium.googlesource.com/chromium/src/+/HEAD/docs/infra/builder_health_indicators.md">Builder Health Documentation</a>.</p>
	</body>
	</html>
	`
			reader, _ := gzip.NewReader(bytes.NewReader(task.BodyGzip))
			decompressedBytes, _ := ioutil.ReadAll(reader)
			finalBody := string(decompressedBytes)
			assert.Loosely(t, finalBody, should.Equal(expectedBody))
		})
	})
}
