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

import { FilterOption } from './types';

export const FILTER_OPTIONS: FilterOption[] = [
  {
    label: 'Status',
    value: 'status',
    nameSpace: 'attributes',
    options: [
      { label: 'Active', value: 'active' },
      { label: 'Inactive', value: 'inactive' },
    ],
  },
  {
    label: 'Priority',
    value: 'priority',
    nameSpace: 'attributes',
    options: [
      { label: 'High', value: 'high' },
      { label: 'Medium', value: 'medium' },
      { label: 'Low', value: 'low' },
    ],
  },
  {
    label: 'Category',
    value: 'category',
    nameSpace: 'attributes',
    options: [
      { label: 'A', value: 'a' },
      { label: 'B', value: 'b' },
      { label: 'C', value: 'c' },
    ],
  },
  {
    label: 'Size',
    value: 'size',
    nameSpace: 'attributes',
    options: [
      { label: 'Small', value: 'small' },
      { label: 'Medium', value: 'medium' },
      { label: 'Large', value: 'large' },
    ],
  },
  {
    label: 'Color',
    value: 'color',
    nameSpace: 'attributes',
    options: [
      { label: 'Red', value: 'red' },
      { label: 'Green', value: 'green' },
      { label: 'Blue', value: 'blue' },
    ],
  },
  {
    label: 'Material',
    value: 'material',
    nameSpace: 'attributes',
    options: [
      { label: 'Wood', value: 'wood' },
      { label: 'Metal', value: 'metal' },
      { label: 'Plastic', value: 'plastic' },
    ],
  },
  {
    label: 'Region',
    value: 'region',
    nameSpace: 'location',
    options: [
      { label: 'North', value: 'north' },
      { label: 'South', value: 'south' },
      { label: 'East', value: 'east' },
      { label: 'West', value: 'west' },
    ],
  },
  {
    label: 'Country',
    value: 'country',
    nameSpace: 'location',
    options: [
      { label: 'USA', value: 'usa' },
      { label: 'Canada', value: 'canada' },
      { label: 'Mexico', value: 'mexico' },
    ],
  },
  {
    label: 'Department',
    value: 'department',
    nameSpace: 'organization',
    options: [
      { label: 'Engineering', value: 'engineering' },
      { label: 'Marketing', value: 'marketing' },
      { label: 'Sales', value: 'sales' },
    ],
  },
  {
    label: 'Team',
    value: 'team',
    nameSpace: 'organization',
    options: [
      { label: 'Alpha', value: 'alpha' },
      { label: 'Beta', value: 'beta' },
      { label: 'Gamma', value: 'gamma' },
    ],
  },
];

export const TEST_FILTER_OPTIONS: FilterOption[] = [
  {
    label: 'Option 1',
    value: 'val-1',
    nameSpace: 'default-namespace',
    options: [
      { label: 'The first option', value: 'o11' },
      { label: 'The second option', value: 'o12' },
    ],
  },
  {
    label: 'Option 2',
    value: 'val-2',
    nameSpace: 'default-namespace',
    options: [
      { label: 'The first option', value: 'o21' },
      { label: 'The second option', value: 'o22' },
    ],
  },
  {
    label: 'Option 3',
    value: 'val-3',
    nameSpace: 'default-namespace',
    options: [
      { label: 'The first option', value: 'o31' },
      { label: 'The second option', value: 'o32' },
    ],
  },
  {
    label: 'Option 4',
    value: 'val-4',
    nameSpace: 'default-namespace',
    options: [
      { label: 'The first option', value: 'o41' },
      { label: 'The second option', value: 'o42' },
    ],
  },
];
