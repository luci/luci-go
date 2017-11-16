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

// Package validation contains methods to install handlers for requests from luci-config related to metadata
// and config validation. More specifically, the package provides functions to define the metadata with which to
// respond to GET metadata requests from luci-config and to define the validator function that performs
// config validation as a response to POST requests from luci-config.
package validation
