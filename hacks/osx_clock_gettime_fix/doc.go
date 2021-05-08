// Copyright 2021 The LUCI Authors.
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

// Package osx_clock_gettime_fix, when linked into a binary, replaces references
// to clock_gettime with a reference to a shim over gettimeofday.
//
// clock_gettime is absent on OSX 10.11 and earlier, but >=go1.16 links to it.
// This hack allows to run binaries compiled with go1.16 on OSX 10.11.
package osx_clock_gettime_fix
