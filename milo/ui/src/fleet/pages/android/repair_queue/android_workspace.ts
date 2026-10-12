// Copyright 2026 The LUCI Authors.
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

import { AndroidPageWorkspace } from '@/fleet/workspaces';
import { AndroidWorkspace } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

/**
 * Maps the UI workspace of an Android page to the AndroidWorkspace enum the
 * Android repair queue RPCs are scoped by.
 */
export const toAndroidWorkspaceEnum = (
  workspace: AndroidPageWorkspace,
): AndroidWorkspace =>
  workspace === 'Pixel'
    ? AndroidWorkspace.ANDROID_WORKSPACE_PIXEL
    : AndroidWorkspace.ANDROID_WORKSPACE_ANDROID;
