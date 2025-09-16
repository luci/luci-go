// Copyright 2025 The LUCI Authors.
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

import { createContext, useContext } from 'react';

const IsDevBuildCtx = createContext<boolean>(false);

export const IsDevBuildProvider = IsDevBuildCtx.Provider;

/**
 * Returns whether this is a local development build. This should only be used
 * for:
 * 1. adding extra logs/assertions in during local development, and/or
 * 2. handling module/file resolving difference between a local development
 *    build and a production build.
 *
 * Note that staging environments, which is often named `${project}-dev`,
 * usually use production builds not local development builds. This hook is not
 * suitable for telling whether this is a staging deployment or a production
 * deployment.
 */
export function useIsDevBuild() {
  return useContext(IsDevBuildCtx);
}
