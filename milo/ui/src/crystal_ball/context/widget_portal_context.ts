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

import { createContext } from 'react';

/**
 * The type of the widget portal context.
 */
export interface WidgetPortalContextType {
  /** The target element where the portal should be rendered. */
  target: HTMLDivElement | null;
  /** Callback to set the open state of the portal. */
  setOpen: (open: boolean) => void;
  /** Whether the portal is currently open. */
  isOpen: boolean;
  /** Whether the portal is currently folded. */
  isFolded: boolean;
  /** Callback to fold the portal. */
  fold: () => void;
  /** Callback to expand the portal. */
  expand: () => void;
}

/**
 * Context for managing the widget portal target and visibility.
 */
export const WidgetPortalContext =
  createContext<WidgetPortalContextType | null>(null);
