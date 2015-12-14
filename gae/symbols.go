// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

type stopErr struct{}

func (stopErr) Error() string { return "stop iteration" }

// Stop is understood by various services to stop iterative processes. Examples
// include datastore.Interface.Run's callback.
var Stop error = stopErr{}
