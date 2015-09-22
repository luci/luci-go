// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package config is a library to access the luci-config service.
//
// There are two backends for the interface presented in the interface.go file.
//
// One backend talks directly to the luci-config service, and the other reads
// from the local filesystem.
// Usually, you should use the remote backend in production, and the filesystem
// backend when developing.
package config
