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

package main

import (
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
)

var (
	instructionsAcked = metric.NewCounter(
		"machine_provider/agent/instructions/acked",
		"Number of instructions received from Machine Provider which were acked.",
		nil,
		// Machine Provider server the instruction was received from.
		field.String("mp"),
		// Swarming server the instruction said to connect to.
		field.String("swarming"),
	)

	instructionsReceived = metric.NewCounter(
		"machine_provider/agent/instructions/received",
		"Number of instructions received from Machine Provider.",
		nil,
		// Machine Provider server the instruction was received from.
		field.String("mp"),
		// Swarming server the instruction said to connect to.
		field.String("swarming"),
	)
)
