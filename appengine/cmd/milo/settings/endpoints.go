// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"golang.org/x/net/context"
)

// Service is the endpoint API.
type Service struct{}

// Settings - Updates or returns the user settings.
func (ss *Service) Settings(c context.Context, u *updateReq) (*resp.Settings, error) {
	// TODO(hinoka): Attach an xsrf token.
	return settingsImpl(c, "foo", u)
}
