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

import { SemanticStatusType } from '@/common/styles/status_styles';
import { TestResult_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

export interface DrawerTreeNode {
  id: string;
  label: string;
  level: number;
  children?: DrawerTreeNode[];
  totalTests?: number; // For folder nodes: total tests/variants underneath
  failedTests?: number; // For folder nodes: total failed tests/variants underneath
  isLeaf?: boolean;
  // For leaf nodes that represent a specific test variant
  testId?: string;
  variantHash?: string;
  status?: TestResult_Status; // The primary TestResult_Status (v2) for variant leaves, used for main icon
  isClickable?: boolean; // True if clicking the node should navigate or perform an action
  // For displaying an additional tag (e.g., "flaky", "X failed")
  tag?: string;
  tagColor?: SemanticStatusType; // The SemanticStatusType for styling the tag
}
