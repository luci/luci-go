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

package gs

import (
	"context"
	"errors"
	"testing"

	gs "cloud.google.com/go/storage"
	"github.com/golang/mock/gomock"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// newTestProdClient creates a new Client instance that uses the given
// baseClient for testing.
func newTestProdClient(ctx context.Context, baseClient gcsClient) (Client, error) {
	c := prodClient{
		ctx:        ctx,
		baseClient: baseClient,
	}
	return &c, nil
}

func TestProdClient(t *testing.T) {
	t.Parallel()

	ftt.Run("prodClient", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		t.Run("Attrs", func(t *ftt.Test) {
			t.Run("path has no bucket", func(t *ftt.Test) {
				mockClient := NewMockGcsClient(ctrl)
				client, err := newTestProdClient(ctx, mockClient)
				assert.Loosely(t, err, should.BeNil)

				_, err = client.Attrs("object-only")
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.Equal("path has no bucket"))
			})

			t.Run("path has no filename", func(t *ftt.Test) {
				mockClient := NewMockGcsClient(ctrl)
				client, err := newTestProdClient(ctx, mockClient)
				assert.Loosely(t, err, should.BeNil)

				_, err = client.Attrs("gs://bucket/")
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.Equal("path has no filename"))
			})
		})

		t.Run("Objects", func(t *ftt.Test) {
			mockIterator := NewMockGcsObjectIterator(ctrl)
			gomock.InOrder(
				mockIterator.EXPECT().Next().Return(&gs.ObjectAttrs{Name: "object1"}, nil),
				mockIterator.EXPECT().Next().Return(&gs.ObjectAttrs{Name: "object2"}, nil),
				mockIterator.EXPECT().Next().Return(nil, iterator.Done),
			)

			mockBucketHandle := NewMockGcsBucketHandle(ctrl)
			mockBucketHandle.EXPECT().Objects(ctx, &gs.Query{Prefix: "prefix"}).Return(mockIterator)

			mockClient := NewMockGcsClient(ctrl)
			mockClient.EXPECT().Bucket("bucket").Return(mockBucketHandle)

			client, err := newTestProdClient(ctx, mockClient)
			assert.Loosely(t, err, should.BeNil)

			expectedAttrs := []*gs.ObjectAttrs{{Name: "object1"}, {Name: "object2"}}
			attrs, err := client.Objects("gs://bucket/prefix")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, attrs, should.Resemble(expectedAttrs))
		})

		t.Run("FirstObject", func(t *ftt.Test) {
			t.Run("Success", func(t *ftt.Test) {
				mockIterator := NewMockGcsObjectIterator(ctrl)
				mockIterator.EXPECT().Next().Return(&gs.ObjectAttrs{Name: "pattern/object"}, nil)

				mockBucketHandle := NewMockGcsBucketHandle(ctrl)
				mockBucketHandle.EXPECT().Objects(ctx, &gs.Query{MatchGlob: "*pattern*"}).Return(mockIterator)

				mockClient := NewMockGcsClient(ctrl)
				mockClient.EXPECT().Bucket("bucket").Return(mockBucketHandle)

				client, err := newTestProdClient(ctx, mockClient)
				assert.Loosely(t, err, should.BeNil)

				expectedAttr := &gs.ObjectAttrs{Name: "pattern/object"}
				attr, err := client.FirstObject("bucket", "*pattern*")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, attr, should.Resemble(expectedAttr))
			})

			t.Run("Not found", func(t *ftt.Test) {
				mockIterator := NewMockGcsObjectIterator(ctrl)
				mockIterator.EXPECT().Next().Return(nil, iterator.Done)

				mockBucketHandle := NewMockGcsBucketHandle(ctrl)
				mockBucketHandle.EXPECT().Objects(ctx, &gs.Query{MatchGlob: "*pattern*"}).Return(mockIterator)

				mockClient := NewMockGcsClient(ctrl)
				mockClient.EXPECT().Bucket("bucket").Return(mockBucketHandle)

				client, err := newTestProdClient(ctx, mockClient)
				assert.Loosely(t, err, should.BeNil)

				_, err = client.FirstObject("bucket", "*pattern*")
				assert.Loosely(t, err, should.Equal(NoObjectFoundForPattern))
			})

			t.Run("Underlying GS client error", func(t *ftt.Test) {
				expectedErr := errors.New("gcs error")
				mockIterator := NewMockGcsObjectIterator(ctrl)
				mockIterator.EXPECT().Next().Return(nil, expectedErr)

				mockBucketHandle := NewMockGcsBucketHandle(ctrl)
				mockBucketHandle.EXPECT().Objects(ctx, &gs.Query{MatchGlob: "*pattern*"}).Return(mockIterator)

				mockClient := NewMockGcsClient(ctrl)
				mockClient.EXPECT().Bucket("bucket").Return(mockBucketHandle)

				client, err := newTestProdClient(ctx, mockClient)
				assert.Loosely(t, err, should.BeNil)

				_, err = client.FirstObject("bucket", "*pattern*")
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring(expectedErr.Error()))
			})
		})

		t.Run("SignedURL", func(t *ftt.Test) {
			t.Run("Success", func(t *ftt.Test) {
				mockBucketHandle := NewMockGcsBucketHandle(ctrl)
				mockBucketHandle.EXPECT().SignedURL("object", nil).Return("signed-url", nil)

				mockClient := NewMockGcsClient(ctrl)
				mockClient.EXPECT().Bucket("bucket").Return(mockBucketHandle)

				client, err := newTestProdClient(ctx, mockClient)
				assert.Loosely(t, err, should.BeNil)

				url, err := client.SignedURL("gs://bucket/object", nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, url, should.Equal("signed-url"))
			})

			t.Run("Error", func(t *ftt.Test) {
				expectedErr := errors.New("signed url error")
				mockBucketHandle := NewMockGcsBucketHandle(ctrl)
				mockBucketHandle.EXPECT().SignedURL("object", nil).Return("", expectedErr)

				mockClient := NewMockGcsClient(ctrl)
				mockClient.EXPECT().Bucket("bucket").Return(mockBucketHandle)

				client, err := newTestProdClient(ctx, mockClient)
				assert.Loosely(t, err, should.BeNil)

				_, err = client.SignedURL("gs://bucket/object", nil)
				assert.Loosely(t, err, should.Equal(expectedErr))
			})
		})
	})
}
