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

import { useContext, useEffect, useId } from 'react';

import { PageConfigCtx } from './page_config_state_provider';

/**
 * Returns a dispatch function to toggle the page config dialog if there are
 * page specific configs.
 */
export function useSetShowPageConfig() {
  const ctx = useContext(PageConfigCtx);
  if (ctx === null) {
    throw new Error(
      'useSetShowPageConfig can only be used in a PageConfigProvider',
    );
  }

  return ctx.hasConfig ? ctx.setShowConfigDialog : null;
}

/**
 * Declares that the page has page-specific configs.
 *
 * @throws when there's already another mounted component that has page-specific
 * configs.
 *
 * @returns a boolean that indicates whether the config dialog should be
 * rendered, and a function to update it.
 */
export function usePageSpecificConfig() {
  const ctx = useContext(PageConfigCtx);
  if (ctx === null) {
    throw new Error(
      'usePageSpecificConfig can only be used in a PageConfigProvider',
    );
  }

  const componentId = useId();

  useEffect(() => {
    ctx.setCurrentPageId(componentId);
    return () => ctx.setCurrentPageId(null);
  }, [ctx, componentId]);

  return [ctx.showConfigDialog, ctx.setShowConfigDialog] as const;
}
