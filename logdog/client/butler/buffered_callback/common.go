// Copyright 2018 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package buffered_callback

import (
	"fmt"
)

// Errors on which we panic.
var (
	// Shared.
	InvalidStreamType = fmt.Errorf("wrong StreamType")

	// Datagram-specific.
	LostDatagramChunk = fmt.Errorf(
		"got self-contained Datagram LogEntry while buffered LogEntries exist",
	)

	// Text-specific.
	PartialLineNotLast = fmt.Errorf("partial line not last in LogEntry")
)
