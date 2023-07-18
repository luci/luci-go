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

package acl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config_service/internal/model"
)

// CanReadServices checks whether the requester can read the provided services.
//
// Returns a bitmap that maps to the provided services.
func CanReadServices(ctx context.Context, services []string) ([]bool, error) {
	if len(services) == 0 {
		return nil, errors.New("expected non-empty service list, got empty")
	}
	ret := make([]bool, len(services))
	aclCfg, err := getACLCfgCached(ctx)
	if err != nil {
		return nil, err
	}
	for i, srv := range services {
		ret[i], err = checkServicePerm(ctx, srv, aclCfg.GetServiceAccessGroup())
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}

// CanReadService checks whether the requester can read the config for the
// provided service.
func CanReadService(ctx context.Context, service string) (bool, error) {
	aclCfg, err := getACLCfgCached(ctx)
	if err != nil {
		return false, err
	}
	switch allowed, err := checkServicePerm(ctx, service, aclCfg.GetServiceAccessGroup()); {
	case err != nil:
		return false, err
	case allowed:
		return allowed, nil
	}

	srv := &model.Service{Name: service}
	switch err := datastore.Get(ctx, srv); {
	case err == datastore.ErrNoSuchEntity:
		return false, nil // Deny access for non-existing service.
	case err != nil:
		return false, fmt.Errorf("failed to load service %q: %w", service, err)
	default:
		for _, access := range srv.Info.GetAccess() {
			switch yes, err := checkAccessEntry(ctx, access); {
			case err != nil:
				return false, err
			case yes:
				return true, nil
			}
		}
		return false, nil
	}
}

// check an access entry defined in the service config.
//
// The allowed value can be found in the proto definition of
// https://pkg.go.dev/go.chromium.org/luci/common/proto/config#Service
func checkAccessEntry(ctx context.Context, access string) (bool, error) {
	if group, ok := strings.CutPrefix(access, "group:"); ok {
		switch isMember, err := auth.IsMember(ctx, group); {
		case err != nil:
			return false, fmt.Errorf("failed to perform membership check for group %q: %w", group, err)
		default:
			return isMember, nil
		}
	}
	if !strings.ContainsRune(access, ':') { // default to user kind
		access = fmt.Sprintf("%s:%s", identity.User, access)
	}
	allowedIdentity, err := identity.MakeIdentity(access)
	if err != nil {
		// Unlikely to happen. This would be validated when importing service
		// config.
		return false, fmt.Errorf("invalid identity %q: %w", access, err)
	}
	return auth.CurrentIdentity(ctx) == allowedIdentity, nil
}

// CanValidateService checks whether the requester can validate the config
// for the provided service.
func CanValidateService(ctx context.Context, service string) (bool, error) {
	aclCfg, err := getACLCfgCached(ctx)
	if err != nil {
		return false, err
	}
	// No actions are allowed if no read access to the service.

	switch allowed, err := checkServicePerm(ctx, service, aclCfg.GetServiceAccessGroup()); {
	case err != nil:
		return false, err
	case !allowed:
		return false, nil
	}
	return checkServicePerm(ctx, service, aclCfg.GetServiceValidationGroup())
}

// CanReimportService checks whether the requester can reimport the config for
// the provided service.
func CanReimportService(ctx context.Context, service string) (bool, error) {
	aclCfg, err := getACLCfgCached(ctx)
	if err != nil {
		return false, err
	}
	// No actions are allowed if no read access to the service
	switch allowed, err := checkServicePerm(ctx, service, aclCfg.GetServiceAccessGroup()); {
	case err != nil:
		return false, err
	case !allowed:
		return false, nil
	}
	return checkServicePerm(ctx, service, aclCfg.GetServiceReimportGroup())
}

func checkServicePerm(ctx context.Context, service string, group string) (bool, error) {
	if service == "" {
		return false, errors.New("expected non-empty service, got empty")
	}
	switch yes, err := auth.IsMember(ctx, group); {
	case err != nil:
		return false, fmt.Errorf("failed to perform membership check for group %q: %w", group, err)
	default:
		return yes, nil
	}
}
