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

// Package main contains the entry point code for the LUCI Deployment Tool
// ("luci_deploy"). This tool is a command-line interface designed to perform
// controlled and automated deployment of LUCI (and LUCI-compatible) services.
//
// "luci_deploy" is Linux-oriented, but may also work on other platforms. It
// leverages external tooling for many remote operations; it is the
// responsibility of the user to have suitable versions of that tooling
// installed and available on PATH.
package main
