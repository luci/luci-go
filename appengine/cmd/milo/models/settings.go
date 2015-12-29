// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package models

import "github.com/luci/luci-go/server/auth/identity"

// UserConfig keys off of an identity and stores user configuration.
type UserConfig struct {
	UserID identity.Identity `gae:"$id"`
	Theme  string
}
