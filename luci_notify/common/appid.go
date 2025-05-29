// Copyright 2022 The LUCI Authors.
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

// Package common includes any common utility functions that are used from
// multiple other packages in the project.
package common

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
)

// GetAppID returns the App ID for this service, or an error if we fail to get
// it. It should only ever fail if the context isn't configured properly.
func GetAppID(ctx context.Context) (string, error) {
	signer := auth.GetSigner(ctx)
	if signer == nil {
		return "", errors.New("failed to get the Signer instance for the service")
	}

	info, err := signer.ServiceInfo(ctx)
	if err != nil {
		return "", errors.Fmt("failed to get service info: %w", err)
	}

	return info.AppID, nil
}

// SetAppIDForTest installs a test Signer implementation into the context, with
// the given app ID.
func SetAppIDForTest(c context.Context, appID string) context.Context {
	return auth.ModifyConfig(c, func(cfg auth.Config) auth.Config {
		cfg.Signer = signingtest.NewSigner(&signing.ServiceInfo{
			AppID: appID,
		})
		return cfg
	})
}
