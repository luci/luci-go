// Copyright 2018 The LUCI Authors.
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

package projectscope

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.chromium.org/luci/common/gcloud/iam"
	"go.chromium.org/luci/server/auth"
)

func generateDisplayName(identity *ScopedIdentity) (string, error) {
	if identity.Service == "" {
		return "", fmt.Errorf("service name is empty")
	}
	if identity.Project == "" {
		return "", fmt.Errorf("project name is empty")
	}

	format := struct {
		Service string `json:"luci_svc"`
		Project string `json:"luci_prj"`
	}{
		Service: identity.Service,
		Project: identity.Project,
	}

	bytes, err := json.Marshal(format)
	if err != nil {
		return "", err
	}
	if len(bytes) > iam.MaxServiceAccountDisplayNameUtf8Bytes {
		bytes = bytes[:iam.MaxServiceAccountDisplayNameUtf8Bytes]
	}
	return string(bytes), nil
}

type ServiceAccountClient interface {
	CreateServiceAccount(ctx context.Context, gcpProject, accountid, displayName string) (*iam.ServiceAccount, error)
}
type ServiceAccountCreator func(ctx context.Context, gcpProject string, identity *ScopedIdentity, client ServiceAccountClient) (*iam.ServiceAccount, bool, error)

func createClient(ctx context.Context) (*iam.Client, error) {
	asSelf, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(iam.OAuthScope))
	if err != nil {
		return nil, err
	}
	client := &iam.Client{Client: &http.Client{Transport: asSelf}}
	return client, nil
}

func CreateServiceAccount(ctx context.Context, gcpProject string, identity *ScopedIdentity, client ServiceAccountClient) (*iam.ServiceAccount, bool, error) {
	var err error
	if client == nil {
		client, err = createClient(ctx)
		if err != nil {
			return nil, false, err
		}
	}
	displayName, err := generateDisplayName(identity)
	if err != nil {
		return nil, false, err
	}
	serviceAccount, err := client.CreateServiceAccount(ctx, gcpProject, identity.AccountId, displayName)
	if err != nil {
		return nil, false, nil
	}
	return serviceAccount, true, nil
}
