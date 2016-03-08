// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskqueueClient

import (
	"google.golang.org/api/taskqueue/v1beta2"
)

var (
	// Scopes is the set of OAuth2 scopes needed to consume task queue tasks.
	//
	// Note that we prefer TaskqueueScope, as it is available in the Google
	// Container Engine UI, rather than the more specific TaskqueueConsumerScope.
	Scopes = []string{
		//taskqueue.TaskqueueConsumerScope,
		taskqueue.TaskqueueScope,
	}
)
