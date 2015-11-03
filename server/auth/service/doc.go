// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package service implements a wrapper around API exposed by auth_service:
// https://github.com/luci/luci-py/tree/master/appengine/auth_service
//
// The main focus is AuthDB replication protocol used to propagate changes
// to database of groups.
package service
