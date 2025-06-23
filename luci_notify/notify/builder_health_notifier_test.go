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

import(
	"context"
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
			expectedBody := "\n\t<html>\n\t<head>\n\t\t<meta charset=\"utf-8\">\n\t</head>\n\t<body>\n\t\t<p>Hello,</p>\n\t\t<p>You are receiving this because builders owned by <strong>test@google.com</strong> contain an unhealthy builder score. <strong>2</strong> of your <strong>4</strong> builders are in bad health.</p>\n\n\t\t<p><strong>Unhealthy Builders:</strong></p>\n\t\t<ul><li><strong><a href=\"https://ci.chromium.org/ui/p/chromium/builders/ci/builder2\">chromium.ci:builder2</a>:</strong><li>Your builder is not healthy with health score of 0</li></li><li><strong><a href=\"https://ci.chromium.org/ui/p/chromium/builders/ci/builder3\">chromium.ci:builder3</a>:</strong><li>Your builder is not healthy</li></li></ul>\n\n\t\t<p><strong>Healthy Builders:</strong></p>\n\t\t<ul><li><strong><a href=\"https://ci.chromium.org/ui/p/chromium/builders/ci/builder2\">chromium.ci:builder2</a>:</strong><li>Your builder is not healthy with health score of 0</li></li><li><strong><a href=\"https://ci.chromium.org/ui/p/chromium/builders/ci/builder3\">chromium.ci:builder3</a>:</strong><li>Your builder is not healthy</li></li></ul>\n\n\t\t<p>For more information on builder health, please see the <a href=\"https://chromium.googlesource.com/chromium/src/+/HEAD/docs/infra/builder_health_indicators.md\">Builder Health Documentation</a>.</p>\n\t</body>\n\t</html>\n\t"
			assert.Loosely(t, string(task.BodyGzip), should.Equal(expectedBody))
		})
	})
}