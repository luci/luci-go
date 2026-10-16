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

import { Chip } from '@mui/material';
import { ReactNode } from 'react';

import {
  buildFilters,
  FilterCategory,
  FilterCategoryBuilder,
} from '@/fleet/components/filters/use_filters';

import { RepairPriorityRule } from './editable_rule_row';

/**
 * Formats a signed point value (priority score or rule weight) for display,
 * e.g. `+350 pts`, `-50 pts`, `0 pts`. Values are kept as strings so int64
 * scores are not truncated.
 */
export const formatPoints = (value?: string | number): string => {
  const valueStr = typeof value === 'number' ? String(value) : value || '0';
  if (!valueStr || valueStr === '0' || valueStr === '-0') {
    return '0 pts';
  }
  if (valueStr.startsWith('-')) {
    const absVal = valueStr.slice(1);
    return `-${absVal} pts`;
  }
  const clean = valueStr.startsWith('+') ? valueStr.slice(1) : valueStr;
  return `+${clean} pts`;
};

export const getBugUrl = (bugId: string): string =>
  `https://b.corp.google.com/issues/${encodeURIComponent(bugId)}`;

export interface ParsedPriorityRule {
  readonly id: string;
  readonly expressionAip160: string;
  readonly weight: string;
  readonly chips: readonly ReactNode[];
}

/**
 * Extracts rule condition chips for breakdown display using native
 * SelectedChip components and AIP-160 buildFilters.
 */
export const extractRuleChips = (
  rawExpr: string,
  filterBuilders?: Record<string, FilterCategoryBuilder<FilterCategory>>,
): ReactNode[] => {
  if (!rawExpr?.trim()) return [];
  if (!filterBuilders) {
    return [
      <Chip
        key="fallback"
        variant="filter"
        label={rawExpr.trim()}
        sx={{ cursor: 'default' }}
      />,
    ];
  }

  try {
    const { filters, hasParseError } = buildFilters(
      filterBuilders,
      () => {},
      rawExpr,
      false,
    );
    const active =
      !hasParseError && filters
        ? Object.values(filters).filter((c) => c.isActive())
        : [];

    if (active.length > 0) {
      return active.map((cat, idx) => (
        <Chip
          key={`${cat.key}-${idx}`}
          variant="filter"
          label={cat.getChipLabel()}
          sx={{
            cursor: 'default',
          }}
        />
      ));
    }
  } catch {
    // Fall through to fallback chip
  }

  return [
    <Chip
      key="fallback"
      variant="filter"
      label={rawExpr.trim()}
      sx={{ cursor: 'default' }}
    />,
  ];
};

/**
 * Pre-parses priority rules into `ParsedPriorityRule` records containing
 * pre-computed native filter chips keyed by rule ID.
 */
export const parsePriorityRules = (
  rules: readonly RepairPriorityRule[],
  filterBuilders?: Record<string, FilterCategoryBuilder<FilterCategory>>,
): ReadonlyMap<string, ParsedPriorityRule> => {
  const map = new Map<string, ParsedPriorityRule>();
  for (const rule of rules) {
    map.set(rule.id, {
      id: rule.id,
      expressionAip160: rule.expressionAip160,
      weight: rule.weight,
      chips: extractRuleChips(rule.expressionAip160, filterBuilders),
    });
  }
  return map;
};
