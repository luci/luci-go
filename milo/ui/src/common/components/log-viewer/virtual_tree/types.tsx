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

/**
 * Represents the fields on the tree data source.
 */
export interface TreeNodeData {
  id: string | number;
  name: string;
  children: TreeNodeData[];
}

/**
 * Provides comprehensive search options.
 */
export interface SearchOptions {
  pattern: string;
  enableRegex?: boolean;
  ignoreCase?: boolean;
  filterOnSearch?: boolean;
}

/**
 * Represents tree match data.
 */
export interface SearchTreeMatch {
  nodeId: string;
}

/**
 * Represents the Virtual Node data with additional tree
 * properties.
 */
export interface TreeData<T extends TreeNodeData> {
  id: string;
  level: number;
  name: string;
  isLeafNode: boolean;
  data: T;
  children: Array<TreeData<T>>;
  isOpen: boolean;
  parent: TreeData<T> | undefined;
}

/**
 * Structure for tree node container data.
 */
export interface TreeNodeContainerData<T extends TreeNodeData> {
  handleNodeToggle: (node: TreeData<T>) => void;
  handleNodeSelect: (node: TreeData<T>) => void;
  treeDataList: Array<TreeData<T>>;
  collapseIcon?: React.ReactNode;
  expandIcon?: React.ReactNode;
}

/**
 * ObjectNode is a node in the logs browser tree sent by the server.
 * It could reference a GCS object, a RBE-CAS artifact or a directory prefix.
 */
export interface ObjectNode {
  id: number;

  /**
   * Immediate filename or dirname.
   */
  name: string;

  /**
   * The url to the resource, it is only set for files, i.e. leaf nodes.
   */
  url?: string;

  /**
   * Length of the object in Bytes.
   */
  size?: number;

  children: ObjectNode[];

  /**
   * Whether the tree should be deeplinked to this node.
   */
  deeplinked?: boolean;

  /**
   * The deeplink path for the node, its the relative path minus the root.
   */
  deeplinkpath?: string;

  /**
   * Whether the node can be viewed in the logs viewer.
   */
  viewingsupported?: boolean;

  /**
   * Indicates if the node is part of a RBE-CAS artifacts tree.
   */
  isRBECAS?: boolean;

  /**
   * Number of log files in the tree. This value is only set in the root node.
   */
  logsCount?: number;

  // UI specific properties:

  /**
   * Whether the node matched the search term.
   */
  searchMatched?: boolean;

  /**
   * Indicates if the node is selected.
   */
  selected?: boolean;
}

/**
 * Defines the text labels used in the tree node.
 */
export interface TreeNodeLabels {
  nonSupportedLeafNodeTooltip: string;
  specialNodeInfoTooltip: string;
}

/**
 * Defines the color props used in the tree node. Uses default if not provided.
 */
export interface TreeNodeColors {
  activeSelectionBackgroundColor?: string;
  deepLinkBackgroundColor?: string;
  defaultBackgroundColor?: string;
  searchMatchBackgroundColor?: string;
  unsupportedColor?: string;
}

export type TreeFontVariant = 'subtitle1' | 'body2' | 'caption';
