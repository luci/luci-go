// Copyright 2017 The LUCI Authors.
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

package cipd

import (
	"strings"

	"go.chromium.org/luci/vpython/api/vpython"
)

// PlatformForPEP425Tag returns the CIPD platform inferred from a given Python
// PEP425 tag.
//
// If the platform could not be determined, an empty string will be returned.
func PlatformForPEP425Tag(t *vpython.PEP425Tag) string {
	switch platSplit := strings.SplitN(t.Platform, "_", 2); platSplit[0] {
	case "linux", "manylinux1":
		// Grab the remainder.
		//
		// Examples:
		// - linux_i686
		// - manylinux1_x86_64
		// - linux_arm64
		cpu := ""
		if len(platSplit) > 1 {
			cpu = platSplit[1]
		}
		switch cpu {
		case "i686":
			return "linux-386"
		case "x86_64":
			return "linux-amd64"
		case "arm64":
			return "linux-arm64"
		case "mipsel", "mips":
			return "linux-mips32"
		case "mips64":
			return "linux-mips64"
		default:
			// All remaining "arm*" get the "armv6l" CIPD platform.
			if strings.HasPrefix(cpu, "arm") {
				return "linux-armv6l"
			}
			return ""
		}

	case "macosx":
		// Grab the last token.
		//
		// Examples:
		// - macosx_10_10_intel
		// - macosx_10_10_i386
		if len(platSplit) == 1 {
			return ""
		}
		suffixSplit := strings.SplitN(platSplit[1], "_", -1)
		switch suffixSplit[len(suffixSplit)-1] {
		case "intel", "x86_64", "fat64", "universal":
			return "mac-amd64"
		case "i386", "fat32":
			return "mac-386"
		default:
			return ""
		}

	case "win32":
		// win32
		return "windows-386"
	case "win":
		// Examples:
		// - win_amd64
		if len(platSplit) == 1 {
			return ""
		}
		switch platSplit[1] {
		case "amd64":
			return "windows-amd64"
		default:
			return ""
		}

	default:
		return ""
	}
}
