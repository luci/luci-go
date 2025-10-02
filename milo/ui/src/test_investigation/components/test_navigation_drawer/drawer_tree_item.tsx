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

import { List } from '@mui/material';
import { useMemo } from 'react';

import {
  TestVerdict_Status,
  testVerdict_StatusToJSON,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb.js';

import { useTestDrawer } from './context';
import { ExpandableListItem } from './expandable_list_item';
import { TestNavigationTreeNode } from './types';

interface DrawerTreeItemProps {
  node: TestNavigationTreeNode;
}

export function DrawerTreeItem({ node }: DrawerTreeItemProps) {
  const {
    expandedNodes,
    toggleNodeExpansion,
    currentTestId,
    currentVariantHash,
    onSelectTestVariant,
  } = useTestDrawer();

  const isExpanded = expandedNodes.has(node.id);
  const hasChildren = node.children && node.children.length > 0;

  const handleItemClick = () => {
    if (hasChildren) {
      toggleNodeExpansion(node.id);
    } else if (node.testVariant !== undefined && onSelectTestVariant) {
      onSelectTestVariant(node.testVariant);
    }
  };

  const testStatusTotals = useMemo(() => {
    return {
      [TestVerdict_Status.FAILED]: node.failedTests,
      [TestVerdict_Status.EXECUTION_ERRORED]: node.errorTests,
      [TestVerdict_Status.PRECLUDED]: node.precludedTests,
      [TestVerdict_Status.FLAKY]: node.flakyTests,
      [TestVerdict_Status.SKIPPED]: node.skippedTests,
      [TestVerdict_Status.PASSED]: node.passedTests,
      [TestVerdict_Status.STATUS_UNSPECIFIED]: node.unknownTests,
    };
  }, [node]);
  const countsArray: { type: TestVerdict_Status; value: number }[] = [];

  for (const [key, count] of Object.entries(testStatusTotals)) {
    const testVerdict: TestVerdict_Status | undefined = parseInt(key);
    countsArray.push({ type: testVerdict, value: count });
  }
  countsArray.sort((a, b) => b.value - a.value);

  const getMostCommonStatus = () => {
    return countsArray[0].type;
  };

  const getStatus = () => {
    if (hasChildren) {
      return getMostCommonStatus();
    } else {
      return node.testVariant?.statusV2;
    }
  };

  const getSecondaryText = () => {
    if (hasChildren) {
      const secondHighestCount =
        countsArray[1].value === 0
          ? ''
          : `, ${countsArray[1].value} ${testVerdict_StatusToJSON(countsArray[1].type)}`;
      return `${countsArray[0].value} ${testVerdict_StatusToJSON(countsArray[0].type)}${secondHighestCount}`;
    } else {
      return node.testVariant?.statusV2
        ? testVerdict_StatusToJSON(node.testVariant?.statusV2)
        : '';
    }
  };
  return (
    <ExpandableListItem
      isExpanded={isExpanded}
      label={node.label}
      status={getStatus()}
      secondaryText={getSecondaryText()}
      totalTests={node.totalTests}
      onClick={handleItemClick}
      isSelected={
        currentTestId === node.testVariant?.testId &&
        currentVariantHash === node.testVariant?.variantHash
      }
      level={node.level}
    >
      {hasChildren && (
        <List component="div" disablePadding dense>
          {node.children!.map((child) => (
            <DrawerTreeItem key={child.id} node={child} />
          ))}
        </List>
      )}
    </ExpandableListItem>
  );
}
