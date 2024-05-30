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

// Package testsupport contains helper functions for testing auth service.
package testsupport

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"

	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/internal/permissions"
)

// BuildTargz builds a tar bundle.
func BuildTargz(files map[string][]byte) []byte {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, body := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0600,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil
		}
		if _, err := tw.Write(body); err != nil {
			return nil
		}
	}
	if err := tw.Flush(); err != nil {
		return nil
	}
	if err := tw.Close(); err != nil {
		return nil
	}
	if err := gzw.Flush(); err != nil {
		return nil
	}
	if err := gzw.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}

// SetTestContextSigner is a helper functionfor unit tests.
//
// It installs a test Signer implementation into the context, with the given
// app ID and service account.
func SetTestContextSigner(ctx context.Context, appID, serviceAccount string) context.Context {
	return auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
		cfg.Signer = signingtest.NewSigner(&signing.ServiceInfo{
			AppID:              appID,
			ServiceAccountName: serviceAccount,
		})
		return cfg
	})
}

// PermissionsDB creates a PermissionsDB for tests.
func PermissionsDB(implicitRootBindings bool) *permissions.PermissionsDB {
	db := permissions.NewPermissionsDB(&configspb.PermissionsConfig{
		Role: []*configspb.PermissionsConfig_Role{
			{
				Name: "role/dev.a",
				Permissions: []*protocol.Permission{
					{
						Name: "luci.dev.p1",
					},
					{
						Name: "luci.dev.p2",
					},
				},
			},
			{
				Name: "role/dev.b",
				Permissions: []*protocol.Permission{
					{
						Name: "luci.dev.p2",
					},
					{
						Name: "luci.dev.p3",
					},
				},
			},
			{
				Name: "role/dev.all",
				Includes: []string{
					"role/dev.a",
					"role/dev.b",
				},
			},
			{
				Name: "role/dev.unused",
				Permissions: []*protocol.Permission{
					{
						Name: "luci.dev.p2",
					},
					{
						Name: "luci.dev.p3",
					},
					{
						Name: "luci.dev.p4",
					},
					{
						Name: "luci.dev.p5",
					},
					{
						Name: "luci.dev.unused",
					},
				},
			},
			{
				Name: "role/implicitRoot",
				Permissions: []*protocol.Permission{
					{
						Name: "luci.dev.implicitRoot",
					},
				},
			},
		},
		Attribute: []string{"a1", "a2", "root"},
	}, &config.Meta{
		Path:     "permissions.cfg",
		Revision: "123",
	})
	db.ImplicitRootBindings = func(s string) []*realmsconf.Binding { return nil }
	if implicitRootBindings {
		db.ImplicitRootBindings = func(projectID string) []*realmsconf.Binding {
			return []*realmsconf.Binding{
				{
					Role:       "role/implicitRoot",
					Principals: []string{fmt.Sprintf("project:%s", projectID)},
				},
				{
					Role:       "role/implicitRoot",
					Principals: []string{"group:root"},
					Conditions: []*realmsconf.Condition{
						{
							Op: &realmsconf.Condition_Restrict{
								Restrict: &realmsconf.Condition_AttributeRestriction{
									Attribute: "root",
									Values:    []string{"yes"},
								},
							},
						},
					},
				},
			}
		}
	}
	return db
}
