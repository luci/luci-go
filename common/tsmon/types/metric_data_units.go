// Copyright 2016 The LUCI Authors.
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

// MetricDataUnits are enums for the units of metrics data.
type MetricDataUnits string

// Units of metrics data.
const (
	Unknown      = ""
	Seconds      = "s"
	Milliseconds = "ms"
	Microseconds = "us"
	Nanoseconds  = "ns"
	Bits         = "B"
	Bytes        = "By"

	Kilobytes = "kBy"  // 1000 bytes (not 1024).
	Megabytes = "MBy"  // 1e6 (1,000,000) bytes.
	Gigabytes = "GBy"  // 1e9 (1,000,000,000) bytes.
	Kibibytes = "kiBy" // 1024 bytes.
	Mebibytes = "MiBy" // 1024^2 (1,048,576) bytes.
	Gibibytes = "GiBy" // 1024^3 (1,073,741,824) bytes.

	// * Extended Units
	AmpUnit           = "A"
	MilliampUnit      = "mA"
	DegreeCelsiusUnit = "Cel"
)

// IsSpecified returns true if a unit annotation has been specified for a given
// metric.
func (units MetricDataUnits) IsSpecified() bool {
	return units != ""
}
