// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"fmt"
)

// ToUserID converts user name to user id used in datastore.
func ToUserID(userName string) string {
	return fmt.Sprintf("user:%v", userName)
}
