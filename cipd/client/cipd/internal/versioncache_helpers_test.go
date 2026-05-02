// Copyright 2026 The LUCI Authors.
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

package internal

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/common"
)

func shouldAddTag(ctx context.Context, service, pkg, tag, iid string) comparison.Func[*VersionCache] {
	return func(vc *VersionCache) *failure.Summary {
		pin := common.Pin{PackageName: pkg, InstanceID: iid}
		return should.ErrLike(nil)(vc.AddTag(ctx, service, pin, tag))
	}
}

func shouldHaveTag(ctx context.Context, service, pkg, tag, iid string) comparison.Func[*VersionCache] {
	return func(vc *VersionCache) *failure.Summary {
		pin, err := vc.ResolveTag(ctx, service, pkg, tag)
		if err != nil {
			return should.ErrLike(nil)(err)
		}
		return should.Match(common.Pin{PackageName: pkg, InstanceID: iid})(pin)
	}
}

func shouldNotHaveTag(ctx context.Context, service, pkg, tag string) comparison.Func[*VersionCache] {
	return func(vc *VersionCache) *failure.Summary {
		pin, err := vc.ResolveTag(ctx, service, pkg, tag)
		if err != nil {
			return should.ErrLike(nil)(err)
		}
		return should.Match(common.Pin{})(pin)
	}
}

func shouldAddFile(ctx context.Context, service, pkg, pkgIID, fileName, objIID string) comparison.Func[*VersionCache] {
	return func(vc *VersionCache) *failure.Summary {
		pin := common.Pin{PackageName: pkg, InstanceID: pkgIID}
		return should.ErrLike(nil)(vc.AddExtractedObjectRef(ctx, service, pin, fileName, common.InstanceIDToObjectRef(objIID)))
	}
}

func shouldHaveFile(ctx context.Context, service, pkg, pkgIID, fileName, objIID string) comparison.Func[*VersionCache] {
	return func(vc *VersionCache) *failure.Summary {
		pin := common.Pin{PackageName: pkg, InstanceID: pkgIID}
		ref, err := vc.ResolveExtractedObjectRef(ctx, service, pin, fileName)
		if err != nil {
			return should.ErrLike(nil)(err)
		}
		return should.Match(common.InstanceIDToObjectRef(objIID))(ref)
	}
}

func shouldNotHaveFile(ctx context.Context, service, pkg, pkgIID, fileName string) comparison.Func[*VersionCache] {
	return func(vc *VersionCache) *failure.Summary {
		pin := common.Pin{PackageName: pkg, InstanceID: pkgIID}
		ref, err := vc.ResolveExtractedObjectRef(ctx, service, pin, fileName)
		if err != nil {
			return should.ErrLike(nil)(err)
		}
		return should.BeNil(ref)
	}
}

func shouldAddRef(ctx context.Context, service, pkg, tag, iid string) comparison.Func[*VersionCache] {
	return func(vc *VersionCache) *failure.Summary {
		pin := common.Pin{PackageName: pkg, InstanceID: iid}
		return should.ErrLike(nil)(vc.AddRef(ctx, service, pin, tag))
	}
}

func shouldHaveRef(ctx context.Context, service, pkg, tag, iid string) comparison.Func[*VersionCache] {
	return func(vc *VersionCache) *failure.Summary {
		pin, err := vc.ResolveRef(ctx, service, pkg, tag)
		if err != nil {
			return should.ErrLike(nil)(err)
		}
		return should.Match(common.Pin{PackageName: pkg, InstanceID: iid})(pin)
	}
}

func shouldNotHaveRef(ctx context.Context, service, pkg, tag string) comparison.Func[*VersionCache] {
	return func(vc *VersionCache) *failure.Summary {
		pin, err := vc.ResolveRef(ctx, service, pkg, tag)
		if err != nil {
			return should.ErrLike(nil)(err)
		}
		return should.Match(common.Pin{})(pin)
	}
}

// IID1 is an old style sha1 instanceid.
func IID1(letter string) string {
	return strings.Repeat(letter, 40)
}

// IID2 is new-style sha256 instanceid.
func IID2(printable any) string {
	return common.ObjectRefToInstanceID(ORef(printable))
}

// ORef is a new-style ObjectRef.
func ORef(printable any) *caspb.ObjectRef {
	h := common.MustNewHash(common.DefaultHashAlgo)
	fmt.Fprint(h, printable)
	return common.ObjectRefFromHash(h)
}
