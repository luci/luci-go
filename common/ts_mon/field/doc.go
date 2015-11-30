// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package field contains constructors for metric field definitions.
//
// When you create a metric you must specify the names and types of the fields
// that can appear on that metric.  When you increment the metric's value you
// must then pass values for all the fields you defined earlier.
package field
