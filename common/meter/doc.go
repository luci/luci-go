// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package meter provides a generic work bundling capability. After a meter is
// configured and started, work units are added to it externally. The meter
// buffers the work that it receives until a flushing threshold (time, count,
// or external) is met, at which point it exports the collection of buffered
// work.
package meter
