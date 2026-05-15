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

package cipd

import (
	"context"
	"strings"
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/common"
)

var set = stringset.NewFromSlice

func TestPackageVersionSet(t *testing.T) {
	t.Parallel()

	m := PackageVersionSet{}

	srv1 := "some.server.example.com"
	srv2 := "other.server.example.com"

	aIID := strings.Repeat("a", 40)

	m.Add(srv1, "some/pkg", "latest")
	m.Add(srv1, "some/pkg", "greatest")
	m.Add(srv1, "some/pkg", "most/fantastic")
	m.Add(srv2, "some/pkg", "tag:123")
	m.Add(srv1, "some/pkg", aIID)

	assert.That(t, m, should.Match(PackageVersionSet{
		{srv1, "some/pkg", "latest"}:         {},
		{srv1, "some/pkg", "greatest"}:       {},
		{srv1, "some/pkg", "most/fantastic"}: {},
		{srv2, "some/pkg", "tag:123"}:        {},
		{srv1, "some/pkg", aIID}:             {},
	}))

	vc := &internal.VersionCache{
		Tags:           internal.Passthrough,
		FileObjectRefs: internal.Passthrough,
		Refs:           internal.Passthrough,
	}

	// We have a tag and ref in here; we should get an error.
	_, err := m.resolve(t.Context(), vc, func(ctx context.Context, service, pkg, version string) (common.Pin, error) {
		return common.Pin{}, errors.New("no resolution")
	}, nil)
	assert.ErrIsLike(t, err, "no resolution")

	bIID := strings.Repeat("b", 40)

	// Give it a fake resolver which always resolves to "bbb...bbb"
	pins, err := m.resolve(t.Context(), vc, func(ctx context.Context, service, pkg, version string) (common.Pin, error) {
		return common.Pin{
			PackageName: pkg,
			InstanceID:  bIID,
		}, nil
	}, nil)
	assert.NoErr(t, err)
	assert.That(t, pins, should.Match(PackageVersionSet{
		{srv1, "some/pkg", strings.Repeat("a", 40)}: {},
		{srv1, "some/pkg", strings.Repeat("b", 40)}: {},
		{srv2, "some/pkg", strings.Repeat("b", 40)}: {},
	}))

	// And our version cache contains all resolved versions.
	for _, ref := range []string{"latest", "greatest", "most/fantastic"} {
		pin, err := vc.ResolveRef(t.Context(), srv1, "some/pkg", ref)
		assert.NoErr(t, err)
		assert.That(t, pin, should.Match(common.Pin{
			PackageName: "some/pkg",
			InstanceID:  bIID,
		}))
	}
	pin, err := vc.ResolveTag(t.Context(), srv2, "some/pkg", "tag:123")
	assert.NoErr(t, err)
	assert.That(t, pin, should.Match(common.Pin{
		PackageName: "some/pkg",
		InstanceID:  bIID,
	}))
}

func TestOfflineResolve(t *testing.T) {
	t.Parallel()

	t.Run(`ok`, func(t *testing.T) {
		t.Parallel()

		ef, err := ensure.ParseFile(strings.NewReader(`
$ServiceURL https://server.example.com/
$VerifiedPlatform linux-amd64 linux-arm64 macos-arm64

some/package/${platform} tag:123
some/other/package latest
`))
		assert.NoErr(t, err)

		// Prepare a version file with only two resolved versions.
		vf := ensure.VersionsFile{}
		vf.AddVersion("some/package/linux-amd64", "tag:123", strings.Repeat("a", 40))
		vf.AddVersion("some/other/package", "latest", strings.Repeat("b", 40))

		m, err := OfflineResolve(ClientOptions{}, ef, vf)
		assert.NoErr(t, err)

		assert.That(t, m, should.Match(PackageVersionSet{
			{"https://server.example.com", "some/other/package", strings.Repeat("b", 40)}:       {},
			{"https://server.example.com", "some/package/linux-amd64", strings.Repeat("a", 40)}: {},
			{"https://server.example.com", "some/package/linux-arm64", "tag:123"}:               {},
			{"https://server.example.com", "some/package/macos-arm64", "tag:123"}:               {},
		}))
	})

	t.Run(`defaultServiceURL`, func(t *testing.T) {
		ef, err := ensure.ParseFile(strings.NewReader(`
some/package/linux-amd64 tag:123
`))
		assert.NoErr(t, err)

		m, err := OfflineResolve(ClientOptions{
			ServiceURL: "http://localhost:1234",
		}, ef, nil)
		assert.NoErr(t, err)

		assert.That(t, m, should.Match(PackageVersionSet{
			{"http://localhost:1234", "some/package/linux-amd64", "tag:123"}: {},
		}))
	})
}
