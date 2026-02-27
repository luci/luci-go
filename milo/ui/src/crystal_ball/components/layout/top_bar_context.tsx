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

import { createContext, useContext, ReactNode, useEffect } from 'react';

/**
 * Context type for the TopBar configuration.
 */
interface TopBarContextType {
  /**
   * The current title displayed in the TopBar, separarted by a pipe from the logo.
   */
  title: ReactNode | null;
  /**
   * Function to update the TopBar title.
   */
  setTitle: (title: ReactNode | null) => void;
  /**
   * The current actions displayed in the TopBar (right side).
   */
  actions: ReactNode | null;
  /**
   * Function to update the TopBar actions.
   */
  setActions: (actions: ReactNode | null) => void;
  /**
   * Extra menu items displayed in the vertical ... menu.
   */
  menuItems: ReactNode | null;
  /**
   * Function to update the TopBar menu items.
   */
  setMenuItems: (items: ReactNode | null) => void;
}

/**
 * Context for managing the TopBar state.
 */
export const TopBarContext = createContext<TopBarContextType | undefined>(
  undefined,
);

/**
 * Hook to access the TopBar context.
 * Must be used within a TopBarProvider.
 *
 * @throws {Error} If used outside of a TopBarProvider.
 * @returns {TopBarContextType} The TopBar context value.
 */
export function useTopBar() {
  const context = useContext(TopBarContext);
  if (context === undefined) {
    throw new Error('useTopBar must be used within a TopBarProvider');
  }
  return context;
}

/**
 * Hook to set the page title and actions in the TopBar.
 * Resets them when the component unmounts.
 */
export function useTopBarConfig(
  title: ReactNode,
  actions?: ReactNode,
  menuItems?: ReactNode,
) {
  const { setTitle, setActions, setMenuItems } = useTopBar();

  useEffect(() => {
    setTitle(title);
    setActions(actions || null);
    setMenuItems(menuItems || null);
    return () => {
      setTitle(null);
      setActions(null);
      setMenuItems(null);
    };
  }, [title, actions, menuItems, setTitle, setActions, setMenuItems]);
}
