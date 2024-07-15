// Copyright 2024 The LUCI Authors.
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

import { Invocation_State } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';

export const INVOCATION_STATE_DISPLAY_MAP = Object.freeze({
  [Invocation_State.STATE_UNSPECIFIED]: 'unspecified',
  [Invocation_State.ACTIVE]: 'active',
  [Invocation_State.FINALIZING]: 'finalizing',
  [Invocation_State.FINALIZED]: 'finalized',
});
