// Copyright 2024 The LUCI Authors.
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

package model

import (
	"context"
	"fmt"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
)

func getServiceIdentity(ctx context.Context) (identity.Identity, error) {
	serviceAccountName, err := getServiceAccountName(ctx)
	if err != nil {
		return identity.AnonymousIdentity, err
	}
	return identity.Identity(
		fmt.Sprintf("%s:%s", identity.User, serviceAccountName),
	), nil
}

func getServiceAccountName(ctx context.Context) (string, error) {
	signer := auth.GetSigner(ctx)
	if signer == nil {
		return "", errors.New("no signer for the service")
	}

	info, err := signer.ServiceInfo(ctx)
	if err != nil {
		return "", errors.Fmt("failed to get service info: %w", err)
	}

	return info.ServiceAccountName, nil
}
