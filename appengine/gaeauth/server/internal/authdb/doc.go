// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package authdb implements datastore-based storage and update of AuthDB
// snapshots used for authorization decisions by server/auth/*.
//
// It uses server/auth/service to communicate with auth_service to fetch AuthDB
// snapshots and subscribe to PubSub notifications.
package authdb
