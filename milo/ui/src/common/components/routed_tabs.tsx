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

import styled from '@emotion/styled';

import { RoutedTab, RoutedTabs } from '@/generic_libs/components/routed_tabs';

/**
 * A styled `<RoutedTabs />` component, with reduced height and added bottom
 * border.
 */
export const AppRoutedTabs = styled(RoutedTabs)({
  minHeight: '36px',
  borderBottom: '1px solid var(--divider-color)',
});

/**
 * A styled `<RoutedTab />` component. Should be placed in `<RoutedTabs />`.
 */
export const AppRoutedTab = styled(RoutedTab)({
  color: 'var(--default-text-color)',
  padding: '0 14px',
  minHeight: '36px',
});
