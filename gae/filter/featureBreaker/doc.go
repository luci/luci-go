// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package featureBreaker contains filters for dynamically disabling/breaking
// API features at test-time.
//
// In particular, it can be used to cause specific service methods to start
// returning specific errors during the test.
package featureBreaker
