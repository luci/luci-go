// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package gs implements a versatile Google Storage client on top of the
// standard Google Storage Go API. It adds:
//	- The ability to read from specific byte offsets.
//	- Exponential backoff retries on transient errors.
//	- Logging
//	- The ability to easily stub a Google Storage interface.
package gs
