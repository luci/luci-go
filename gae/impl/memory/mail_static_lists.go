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

package memory

import (
	"go.chromium.org/luci/common/data/stringset"
)

// all of these constants were imported on Mon Dec 14 18:18:00 PST 2015 from
// https://cloud.google.com/appengine/docs/go/mail/

var okHeaders = stringset.NewFromSlice(
	"In-Reply-To",
	"List-Id",
	"List-Unsubscribe",
	"On-Behalf-Of",
	"References",
	"Resent-Date",
	"Resent-From",
	"Resent-To",
)

var okMimeTypes = stringset.NewFromSlice(
	"application/msword",
	"application/pdf",
	"application/rss+xml",
	"application/vnd.google-earth.kml+xml",
	"application/vnd.google-earth.kmz",
	"application/vnd.ms-excel",
	"application/vnd.ms-powerpoint",
	"application/vnd.oasis.opendocument.presentation",
	"application/vnd.oasis.opendocument.spreadsheet",
	"application/vnd.oasis.opendocument.text",
	"application/vnd.sun.xml.calc",
	"application/vnd.sun.xml.writer",
	"application/x-gzip",
	"application/zip",
	"audio/basic",
	"audio/flac",
	"audio/mid",
	"audio/mp4",
	"audio/mpeg",
	"audio/ogg",
	"audio/x-aiff",
	"audio/x-wav",
	"image/gif",
	"image/jpeg",
	"image/png",
	"image/tiff",
	"image/vnd.wap.wbmp",
	"image/x-ms-bmp",
	"text/calendar",
	"text/comma-separated-values",
	"text/css",
	"text/html",
	"text/plain",
	"text/x-vcard",
	"video/mp4",
	"video/mpeg",
	"video/ogg",
	"video/quicktime",
	"video/x-msvideo",
)

var badExtensions = stringset.NewFromSlice(
	"ade",
	"adp",
	"bat",
	"chm",
	"cmd",
	"com",
	"cpl",
	"exe",
	"hta",
	"ins",
	"isp",
	"jse",
	"lib",
	"mde",
	"msc",
	"msp",
	"mst",
	"pif",
	"scr",
	"sct",
	"shb",
	"sys",
	"vb",
	"vbe",
	"vbs",
	"vxd",
	"wsc",
	"wsf",
	"wsh",
)

var extensionMapping = map[string]string{
	"aif":  "audio/x-aiff",
	"aifc": "audio/x-aiff",
	"aiff": "audio/x-aiff",
	"asc":  "text/plain",
	"au":   "audio/basic",
	"avi":  "video/x-msvideo",
	"bmp":  "image/x-ms-bmp",
	"css":  "text/css",
	"csv":  "text/comma-separated-values",
	"diff": "text/plain",
	"doc":  "application/msword",
	"docx": "application/msword",
	"flac": "audio/flac",
	"gif":  "image/gif",
	"gzip": "application/x-gzip",
	"htm":  "text/html",
	"html": "text/html",
	"ics":  "text/calendar",
	"jpe":  "image/jpeg",
	"jpeg": "image/jpeg",
	"jpg":  "image/jpeg",
	"kml":  "application/vnd.google-earth.kml+xml",
	"kmz":  "application/vnd.google-earth.kmz",
	"m4a":  "audio/mp4",
	"mid":  "audio/mid",
	"mov":  "video/quicktime",
	"mp3":  "audio/mpeg",
	"mp4":  "video/mp4",
	"mpe":  "video/mpeg",
	"mpeg": "video/mpeg",
	"mpg":  "video/mpeg",
	"odp":  "application/vnd.oasis.opendocument.presentation",
	"ods":  "application/vnd.oasis.opendocument.spreadsheet",
	"odt":  "application/vnd.oasis.opendocument.text",
	"oga":  "audio/ogg",
	"ogg":  "audio/ogg",
	"ogv":  "video/ogg",
	"pdf":  "application/pdf",
	"png":  "image/png",
	"pot":  "text/plain",
	"pps":  "application/vnd.ms-powerpoint",
	"ppt":  "application/vnd.ms-powerpoint",
	"pptx": "application/vnd.ms-powerpoint",
	"qt":   "video/quicktime",
	"rmi":  "audio/mid",
	"rss":  "application/rss+xml",
	"snd":  "audio/basic",
	"sxc":  "application/vnd.sun.xml.calc",
	"sxw":  "application/vnd.sun.xml.writer",
	"text": "text/plain",
	"tif":  "image/tiff",
	"tiff": "image/tiff",
	"txt":  "text/plain",
	"vcf":  "text/x-vcard",
	"wav":  "audio/x-wav",
	"wbmp": "image/vnd.wap.wbmp",
	"xls":  "application/vnd.ms-excel",
	"xlsx": "application/vnd.ms-excel",
	"zip":  "application/zip",
}
