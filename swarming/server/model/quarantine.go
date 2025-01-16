// Copyright 2025 The LUCI Authors.
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

package model

// GenericQuarantineMessage is returned by QuarantineMessage in case the
// bot sets "quarantine" value to a generic truthy value (instead of a string
// with a detailed explanation).
const GenericQuarantineMessage = "Bot self-quarantined."

// QuarantineMessage interprets "quarantined" state or dimension value.
//
// Returns a string with a quarantine message if the bot has quarantined itself
// or an empty string if the bot is not in self-quarantine.
//
// May either return some custom string (if the bot provided it) or a
// GenericQuarantineMessage if the bot just set the "quarantined" value to
// "true" or equivalent.
func QuarantineMessage(quarantined any) string {
	switch val := quarantined.(type) {
	case nil:
		return ""
	case string:
		return val
	case float64:
		if val != 0 {
			return GenericQuarantineMessage
		}
		return ""
	case bool:
		if val {
			return GenericQuarantineMessage
		}
		return ""
	case []any:
		if len(val) != 0 {
			return GenericQuarantineMessage
		}
		return ""
	default:
		return GenericQuarantineMessage
	}
}
