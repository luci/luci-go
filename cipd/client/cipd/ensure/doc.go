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

// Package ensure contains methods and types for interacting with the 'ensure
// file format'.
//
// The format is used by the cipd client to describe the desired state of a cipd
// installation. This states can be asserted with the cipd client 'ensure'
// command. The state is essentialy a list of packages, their versions and their
// installation subdirectories ("subdirs").
//
// Format Description
//
// The file is line-oriented. All statements fit on a single line.
//
// A line can be blank, a comment, a setting, a package, or a directive.
//
//
// Comments
//
// A comment begins with a # and goes to the end of the line. It is ignored.
// Example:
//   # This is a comment. It has no effect.
//
//
// Settings
//
// A setting looks like `$name value`. Settings are global and can only be set
// once per file. The following settings are allowed:
//   - `$ServiceURL <url>` is the url for the cipd service. It can be used in
//     lieu of the -service-url command line parameter.
//   - `$VerifiedPlatform <platform> [ <platform>]*` allows the manifest to
//     specify a list of ${platform} expansions to use for verification.
//     Multiple $VerifiedPlatform directives can be specified, and will
//     accumulate.
//
//
// Package Definitions
//
// A package line looks like `<package_template> <version>`. Package templates
// are cipd package names, with optional expansion parameters `${os}`,
// `${arch}`, and `${platform}`. These placeholders can appear anywhere in the
// package template except for the first letter.  All other characters in the
// template are taken verbatim.
//
// ${os} will expand to one of the following, based on the value of this
// client's runtime.GOOS value:
//   * windows
//   * mac
//   * linux
//
// ${arch} will expand to one of the following, based on the value of this
// client's runtime.GOARCH value:
//   * 386
//   * amd64
//   * armv6l
//
// Since these two often appear together, the convenience placeholder
// `${platform}` expands to the equivalent of `${os}-${arch}`.
//
// All of these paramters also support the syntax ${var=possible,values}.
// What this means is that the package line will be expanded if, and only if,
// var equals one of the possible values. If that var does not match
// a possible value, the line is ignored. This allows you to do, e.g.:
//   path/to/package/${os=windows}  windows_release
//   path/to/package/${os=linux}    linux_release
//   # no version for mac
//
//   path/to/posix/tool/${os=mac,linux}  some_tag:value
//   # no version for windows
//
//
// Directives
//
// A directive looks like `@name value`. Directives are 'sticky' and apply until
// the next same-name directive. The following directive names are allowed:
//   - `@Subdir` allows you to change the subdirectory that subsequent packages
//     are installed to.
//     * The subdir value is always relative to the root of the cipd
//       installation (the directory containing the .cipd folder).
//     * The value of Subdir before any @Subdir directives is "", or the root
//       of the cipd installation.
//     * You can reset the directory back to root by doing `@Subdir` by itself,
//       without a value.
//     * @Subdir directives also support expansion parameters `${os}`, `${arch}`
//       and `${platform}`. Using a subdir expansion like `${param=val}` will
//       cause that subdirectory, and any packages in it, to only exist if the
//       param matches one of the values.
//
//
// Example
//
// Here is an example ensure file which demonstrates all the various features.
//
//   # This is an ensure file!
//   $ServiceURL https://chrome-infra-packages.appspot.com/
//
//   # This is the cipd client itself
//   infra/tools/cipd/${os}-${arch}  latest
//
//   @Subdir python
//   python/wheels/pip                     version:8.1.2
//   # use the convenience placeholder
//   python/wheels/coverage/${platform}    version:4.1
//
//   @Subdir infra/support
//   infra/some/other/package deadbeefdeadbeefdeadbeefdeadbeefdeadbeef
//
//   # only exists on windows machines
//   @Subdir support/${os=windows}-${arch}
//   some/support/package  latest
//   some/other/support/package  latest
//
//   # Always exists, but the directory changes based on the os
//   @Subdir platform/${os}
//   a/platform/package latest
package ensure
