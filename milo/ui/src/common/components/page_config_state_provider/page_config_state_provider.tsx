// Copyright 2023 The LUCI Authors.
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

import {
  Dispatch,
  SetStateAction,
  createContext,
  useContext,
  useEffect,
  useId,
  useState,
} from 'react';

interface ContextValue {
  readonly setCurrentPageId: (id: string | null) => void;
  readonly hasConfig: boolean;
  readonly showConfigDialog: boolean;
  readonly setShowConfigDialog: Dispatch<SetStateAction<boolean>>;
}

const Context = createContext<ContextValue | null>(null);

export interface PageConfigStateProviderProps {
  readonly children: React.ReactNode;
}

export function PageConfigStateProvider({
  children,
}: PageConfigStateProviderProps) {
  const [pageId, setPageId] = useState<string | null>(null);
  const [showConfigDialog, setShowConfigDialog] = useState(false);

  function setCurrentPageId(newPageId: string | null) {
    // Apply some extra checks to ensure we can't have two page config dialogs
    // at the same time.
    if (pageId !== null && newPageId !== null && pageId !== newPageId) {
      throw new Error(
        'cannot support two page config dialogs at the same time',
      );
    }
    setPageId(newPageId);
  }

  return (
    <Context.Provider
      value={{
        setCurrentPageId,
        hasConfig: pageId !== null,
        showConfigDialog,
        setShowConfigDialog,
      }}
    >
      {children}
    </Context.Provider>
  );
}

/**
 * Returns a dispatch function to toggle the page config dialog if there are
 * page specific configs.
 */
export function useSetShowPageConfig() {
  const ctx = useContext(Context);
  if (ctx === null) {
    throw new Error(
      'useSetShowPageConfig must be used within PageConfigProvider',
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
  const ctx = useContext(Context);
  if (ctx === null) {
    throw new Error(
      'usePageSpecificConfig must be used within PageConfigProvider',
    );
  }

  const componentId = useId();

  useEffect(() => {
    ctx.setCurrentPageId(componentId);
    return () => ctx.setCurrentPageId(null);
  }, [ctx, componentId]);

  return [ctx.showConfigDialog, ctx.setShowConfigDialog] as const;
}
