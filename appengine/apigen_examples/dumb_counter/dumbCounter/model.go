// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dumbCounter

// Crappy datastore model!

// Counter is a stupid model which holds a single numerical value.
type Counter struct {
	Name string `gae:"$id"`

	Val int64 `json:",string"`
}
