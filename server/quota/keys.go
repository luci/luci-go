// Copyright 2022 The LUCI Authors.
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

package quota

import "go.chromium.org/luci/server/quota/internal/quotakeys"

// AssembleASI will return an Application-Specified-Identifier (ASI) with the
// given sections.
//
// Sections are assembled with a "|" separator verbatim, unless the section
// contains a "|", "~" or begins with "{". In this case the section will be
// encoded with ascii85 and inserted to the final string with a "{" prefix
// character.
var AssembleASI = quotakeys.AssembleASI

// DecodeASI will return the sections within an Application-Specified-Identifier
// (ASI), decoding any which appear to be ascii85-encoded (i.e. those prefixed
// with a "{").
//
// If a section has the "{" prefix, but doesn't correctly decode, it's
// returned verbatim.
var DecodeASI = quotakeys.DecodeASI
