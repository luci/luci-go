// Copyright 2019 The LUCI Authors.
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

package protoutil

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestValidateBuilderID(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateBuilderID", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			err := ValidateBuilderID(nil)
			assert.Loosely(t, err, should.ErrLike("project must match"))
		})

		t.Run("empty", func(t *ftt.Test) {
			b := &pb.BuilderID{}
			err := ValidateBuilderID(b)
			assert.Loosely(t, err, should.ErrLike("project must match"))
		})

		t.Run("project", func(t *ftt.Test) {
			b := &pb.BuilderID{}
			err := ValidateBuilderID(b)
			assert.Loosely(t, err, should.ErrLike("project must match"))
		})

		t.Run("bucket", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				b := &pb.BuilderID{
					Project: "project",
				}
				err := ValidateBuilderID(b)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				b := &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket!",
				}
				err := ValidateBuilderID(b)
				assert.Loosely(t, err, should.ErrLike("bucket must match"))
			})

			t.Run("v1", func(t *ftt.Test) {
				b := &pb.BuilderID{
					Project: "project",
					Bucket:  "luci.project.bucket",
				}
				err := ValidateBuilderID(b)
				assert.Loosely(t, err, should.ErrLike("invalid use of v1 bucket in v2 API"))
			})

			t.Run("ok", func(t *ftt.Test) {
				b := &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
				}
				err := ValidateBuilderID(b)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("builder", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				b := &pb.BuilderID{
					Project: "project",
				}
				err := ValidateBuilderID(b)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				b := &pb.BuilderID{
					Project: "project",
					Builder: "builder!",
				}
				err := ValidateBuilderID(b)
				assert.Loosely(t, err, should.ErrLike("builder must match"))
			})

			t.Run("ok", func(t *ftt.Test) {
				b := &pb.BuilderID{
					Project: "project",
					Builder: "builder",
				}
				err := ValidateBuilderID(b)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}

func TestValidateRequiredBuilderID(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateRequiredBuilderID", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			err := ValidateRequiredBuilderID(nil)
			assert.Loosely(t, err, should.ErrLike("project must match"))
		})

		t.Run("empty", func(t *ftt.Test) {
			b := &pb.BuilderID{}
			err := ValidateRequiredBuilderID(b)
			assert.Loosely(t, err, should.ErrLike("project must match"))
		})

		t.Run("project", func(t *ftt.Test) {
			b := &pb.BuilderID{}
			err := ValidateRequiredBuilderID(b)
			assert.Loosely(t, err, should.ErrLike("project must match"))
		})

		t.Run("bucket", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				b := &pb.BuilderID{
					Project: "project",
				}
				err := ValidateRequiredBuilderID(b)
				assert.Loosely(t, err, should.ErrLike("bucket is required"))
			})

			t.Run("invalid", func(t *ftt.Test) {
				b := &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket!",
				}
				err := ValidateRequiredBuilderID(b)
				assert.Loosely(t, err, should.ErrLike("bucket must match"))
			})

			t.Run("v1", func(t *ftt.Test) {
				b := &pb.BuilderID{
					Project: "project",
					Bucket:  "luci.project.bucket",
				}
				err := ValidateRequiredBuilderID(b)
				assert.Loosely(t, err, should.ErrLike("invalid use of v1 bucket in v2 API"))
			})

			t.Run("builder", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					b := &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
					}
					err := ValidateRequiredBuilderID(b)
					assert.Loosely(t, err, should.ErrLike("builder is required"))
				})

				t.Run("invalid", func(t *ftt.Test) {
					b := &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder!",
					}
					err := ValidateRequiredBuilderID(b)
					assert.Loosely(t, err, should.ErrLike("builder must match"))
				})

				t.Run("ok", func(t *ftt.Test) {
					b := &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					}
					err := ValidateRequiredBuilderID(b)
					assert.Loosely(t, err, should.BeNil)
				})
			})
		})
	})
}

func TestBuilderConversion(t *testing.T) {
	t.Parallel()

	ftt.Run("FormatBuilderID", t, func(t *ftt.Test) {
		assert.Loosely(t, FormatBuilderID(&pb.BuilderID{
			Project: "proj",
			Bucket:  "bucket",
			Builder: "builder",
		}),
			should.Equal(
				"proj/bucket/builder"))
	})

	ftt.Run("FormatBucketID", t, func(t *ftt.Test) {
		assert.Loosely(t, FormatBucketID("proj", "bucket"), should.Equal("proj/bucket"))
	})

	ftt.Run("ParseBucketID", t, func(t *ftt.Test) {
		t.Run("valid", func(t *ftt.Test) {
			p, b, err := ParseBucketID("proj/bucket")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, p, should.Equal("proj"))
			assert.Loosely(t, b, should.Equal("bucket"))
		})

		t.Run("invalid", func(t *ftt.Test) {
			_, _, err := ParseBucketID("proj/bucket/bldr")
			assert.Loosely(t, err, should.ErrLike("invalid bucket id; must have 1 slash"))

			_, _, err = ParseBucketID(" / ")
			assert.Loosely(t, err, should.ErrLike("invalid bucket id; project is empty"))

			_, _, err = ParseBucketID("proj/ ")
			assert.Loosely(t, err, should.ErrLike("invalid bucket id; bucket is empty"))
		})
	})
}
