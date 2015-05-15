// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package retry implements retriable task management. It uses a callback
// mechanism through the Retriable interface and uses signaling via the
// retry.Error type to knowwhen an error should be retried.
package retry
