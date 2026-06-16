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

import { Theme } from '@mui/material';

/**
 * Centralized z-index layers for Crystal Ball.
 *
 * Defining z-indices relative to standard MUI theme z-index tokens guarantees
 * that UI overlays stack correctly and consistently.
 */
export const Z_INDEX = {
  /** Local container overlays (e.g., loading spinners, toolbar items). */
  LOCAL_OVERLAY: 10,

  /** Sticky headers and layouts that sit above standard page contents. */
  STICKY_HEADER: (theme: Theme) => theme.zIndex.drawer + 1,

  /** Side dialog drawers. */
  SIDE_DRAWER: (theme: Theme) => theme.zIndex.modal + 10,

  /** Autocomplete popups, dropdowns, and tooltips rendering in front of drawers. */
  DRAWER_POPUP: (theme: Theme) => theme.zIndex.modal + 20,
};
