// Copyright 2015 The LUCI Authors.
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

package types

// MessageIndex is a prefix or stream message index type.
type MessageIndex int64

// ContentType is a MIME-style content type.
type ContentType string

const (
	// ContentTypeText is a stream content type for text streams
	ContentTypeText ContentType = "text/plain"
	// ContentTypeBinary is a stream content type for binary streams.
	ContentTypeBinary = "application/octet-stream"

	// ContentTypeLogdogDatagram is a content type for size-prefixed datagram
	// frame stream.
	ContentTypeLogdogDatagram = "application/x-logdog-datagram"
	// ContentTypeLogdogLog is a LogDog log stream.
	ContentTypeLogdogLog = "application/x-logdog-logs"
	// ContentTypeLogdogIndex is a LogDog log index.
	ContentTypeLogdogIndex = "application/x-logdog-log-index"
)
