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

package ui

// A collection of structs used for defining OS, platform, and device logos.
// Each logo defined in this file should have a matching icon in the
// application's static/logo/ folder.

type LogoBase struct {
	Img string `json:"img,omitempty"`
	Alt string `json:"alt,omitempty"`
}

type Logo struct {
	LogoBase
	// Subtitle is text that goes under the logo.  This is most commonly used
	// to specify the version of the OS or type of device.
	Subtitle string `json:"subtitle,omitempty"`
	// Count specifies how many of the OS or device there are.
	Count int `json:"count,omitempty"`
}

// Define our known base logos
var (
	Windows = LogoBase{
		Img: "windows.svg",
		Alt: "Microsoft Windows",
	}

	OSX = LogoBase{
		Img: "apple.svg",
		Alt: "Apple OSX",
	}

	Ubuntu = LogoBase{
		Img: "ubuntu.svg",
		Alt: "Canonical Ubuntu",
	}

	// These are devices.
	IOS = LogoBase{
		Img: "ios_device.png",
		Alt: "Apple iOS",
	}

	Android = LogoBase{
		Img: "android_device.png",
		Alt: "Google Android",
	}

	ChromeOS = LogoBase{
		Img: "cros_device.png",
		Alt: "Google ChromeOS",
	}
)
