// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gae

type stopErr struct{}

func (stopErr) Error() string { return "stop iteration" }

// Stop is understood by various services to stop iterative processes. Examples
// include datastore.Interface.Run's callback.
var Stop error = stopErr{}
