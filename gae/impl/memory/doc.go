// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package memory provides an implementation of infra/gae/libs/wrapper which
// backs to local memory ONLY. This is useful for unittesting, and is also used
// for the nested-transaction filter implementation.
package memory
