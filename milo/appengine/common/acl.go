// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package common

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/luci_config/server/cfgclient/access"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend"
	"github.com/luci/luci-go/server/auth"
)

// Helper functions for ACL checking.

// IsAllowed checks to see if the user in the context is allowed to access
// the given project.
func IsAllowed(c context.Context, project string) (bool, error) {
	// Get the project, because that's where the ACLs lie.
	err := access.Check(
		c, backend.AsUser,
		cfgtypes.ProjectConfigSet(cfgtypes.ProjectName(project)))
	switch err {
	case nil:
		return true, nil
	case access.ErrNoAccess:
		return false, nil
	default:
		return false, err
	}
}

// IsAllowedInternal is a shorthand for checking to see if the user is a reader
// of a magic project named "chrome".
func IsAllowedInternal(c context.Context) (bool, error) {
	settings := GetSettings(c)
	if settings.Buildbot.InternalReader == "" {
		return false, nil
	}
	return auth.IsMember(c, settings.Buildbot.InternalReader)
}
