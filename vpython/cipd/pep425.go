// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cipd

import (
	"strings"

	"github.com/luci/luci-go/vpython/api/vpython"
)

// PlatformForPEP425Tag returns the CIPD platform inferred from a given Python
// PEP425 tag.
//
// If the platform could not be determined, an empoty string will be returned.
func PlatformForPEP425Tag(t *vpython.Pep425Tag) string {
	switch archSplit := strings.SplitN(t.Arch, "_", 2); archSplit[0] {
	case "linux", "manylinux1":
		// Grab the remainder.
		//
		// Examples:
		// - linux_i686
		// - manylinux1_x86_64
		// - linux_arm64
		cpu := ""
		if len(archSplit) > 1 {
			cpu = archSplit[1]
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
		if len(archSplit) == 1 {
			return ""
		}
		suffixSplit := strings.SplitN(archSplit[1], "_", -1)
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
		if len(archSplit) == 1 {
			return ""
		}
		switch archSplit[1] {
		case "amd64":
			return "windows-amd64"
		default:
			return ""
		}

	default:
		return ""
	}
}
