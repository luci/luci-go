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

package iam

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"go.chromium.org/luci/common/logging"
	"google.golang.org/api/googleapi"
)

// MaxServiceAccountDisplayNameUtf8Bytes represents the maximum UTF-8 bytes
// allowed in the "DisplayName" field of the service account structure.
const MaxServiceAccountDisplayNameUtf8Bytes = 100

// ServiceAccount represents a GCP IAM service account.
type ServiceAccount struct {
	ID             string `json:"id,omitempty"`
	Name           string `json:"name"`
	ProjectID      string `json:"projectId"`
	UniqueID       string `json:"uniqueId"`
	Email          string `json:"email"`
	DisplayName    string `json:"displayName"`
	Oauth2ClientID string `json:"oauth2ClientId"`
	Etag           string `json:"etag"`
}

var (
	// ErrAlreadyExists indicates that the service account already exists.
	ErrAlreadyExists = errors.New("already exists")

	// ErrNotFound indicates that the service account does not exists.
	ErrNotFound = errors.New("not found")
)

// ServiceAccountClient defines the interface towards the service account related API
// of iam.Client.
type ServiceAccountClient interface {
	GetServiceAccount(c context.Context, project, accountID string) (*ServiceAccount, error)
	CreateServiceAccount(c context.Context, project, accountID, displayName string) (*ServiceAccount, error)
}

// GetServiceAccount retrieves the service account information for a given service account.
func (cl *Client) GetServiceAccount(c context.Context, project, accountID string) (*ServiceAccount, error) {
	serviceAccount := ServiceAccount{}

	err := cl.iamAPIRequest(c, fmt.Sprintf("projects/%s/serviceAccounts", url.QueryEscape(project)), "", "GET", nil, &serviceAccount)
	if err != nil {
		logging.WithError(err).Errorf(c, "GCP GetServiceAccount failed")
		if apiErr, _ := err.(*googleapi.Error); apiErr != nil {
			if apiErr.Code == http.StatusNotFound {
				return nil, ErrNotFound
			}
		}
		return nil, err
	}
	return &serviceAccount, nil
}

// CreateServiceAccount creates a service account using IAM's API.
func (cl *Client) CreateServiceAccount(c context.Context, project, accountID, displayName string) (*ServiceAccount, error) {
	serviceAccount := ServiceAccount{}

	body := struct {
		AccountID      string         `json:"accountId"`
		ServiceAccount ServiceAccount `json:"serviceAccount"`
	}{
		AccountID: accountID,
		ServiceAccount: ServiceAccount{
			DisplayName: displayName,
		},
	}

	err := cl.iamAPIRequest(c, fmt.Sprintf("projects/%s/serviceAccounts", url.QueryEscape(project)), "", "POST", &body, &serviceAccount)
	if err != nil {
		logging.WithError(err).Errorf(c, "GCP CreateServiceAccount failed")
		if apiErr, _ := err.(*googleapi.Error); apiErr != nil {
			if apiErr.Code == http.StatusConflict {
				return nil, ErrAlreadyExists
			}
		}
		return nil, err
	}
	return &serviceAccount, nil
}
