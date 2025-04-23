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

package buganizer

import (
	"context"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"
)

// ComponentPermissions contains the results of checking buganizer component access.
type ComponentPermissions struct {
	// Appender is permission to create issues in this component.
	Appender bool
	// IssueDefaultsAppender is permission to add comments to issues in
	// this component.
	IssueDefaultsAppender bool
	// IsArchived returns whether the component is archived. Issues cannot be created
	// in archived components.
	IsArchived bool
}

// NewComponentAccessChecker initialises a new component access checker.
// client is the issue tracker to use, and emailAddress is the email
// address of the user to check access of (this should be the service's
// own email).
func NewComponentAccessChecker(client Client, emailAddress string) *ComponentAccessChecker {
	return &ComponentAccessChecker{
		client:       client,
		emailAddress: emailAddress,
	}
}

type ComponentAccessChecker struct {
	// The issue tracker client.
	client Client
	// The email address to check access of.
	emailAddress string
}

// CheckAccess checks the permissions required to create an issue
// in the specified component.
func (c *ComponentAccessChecker) CheckAccess(ctx context.Context, componentID int64) (ComponentPermissions, error) {
	var err error
	result := ComponentPermissions{}
	result.Appender, err = c.checkSinglePermission(ctx, componentID, false, "appender")
	if err != nil {
		return ComponentPermissions{}, err
	}
	result.IssueDefaultsAppender, err = c.checkSinglePermission(ctx, componentID, true, "appender")
	if err != nil {
		return ComponentPermissions{}, err
	}
	// If we don't have append permission, no point trying to read the component to determine
	// if it has been archived. We might not have permission to see it.
	if result.Appender && result.IssueDefaultsAppender {
		result.IsArchived, err = c.checkIsArchived(ctx, componentID)
		if err != nil {
			return ComponentPermissions{}, err
		}
	}
	return result, nil
}

// checkSinglePermission checks a single permission of a Buganizer component
// ID.  You should typically use checkComponentPermission instead of this
// method.
func (c *ComponentAccessChecker) checkSinglePermission(ctx context.Context, componentID int64, issueDefaults bool, relation string) (bool, error) {
	resource := []string{"components", strconv.Itoa(int(componentID))}
	if issueDefaults {
		resource = append(resource, "issueDefaults")
	}
	automationAccessRequest := &issuetracker.GetAutomationAccessRequest{
		User:         &issuetracker.User{EmailAddress: c.emailAddress},
		Relation:     relation,
		ResourceName: strings.Join(resource, "/"),
	}
	access, err := c.client.GetAutomationAccess(ctx, automationAccessRequest)
	if err != nil {
		logging.Errorf(ctx, "error when checking buganizer component permissions with request:\n%s\nerror:%s", textPBMultiline.Format(automationAccessRequest), err)
		return false, err
	}
	return access.HasAccess, nil
}

func (c *ComponentAccessChecker) checkIsArchived(ctx context.Context, componentID int64) (bool, error) {
	component, err := c.client.GetComponent(ctx, &issuetracker.GetComponentRequest{
		ComponentId: componentID,
	})
	if err != nil {
		logging.Errorf(ctx, "error when reading component %v to determine if it is archived: %s", componentID, err)
		return false, err
	}
	return component.IsArchived, nil
}
