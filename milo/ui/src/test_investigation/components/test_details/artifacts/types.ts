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

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

export type SelectedArtifactSource = 'result' | 'invocation';

export interface ArtifactTreeNodeData {
  id: string;

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

  children: ArtifactTreeNodeData[];

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
  viewingSupported?: boolean;

  // UI specific properties:

  /**
   * Whether the node matched the search term.
   */
  searchMatched?: boolean;

  /**
   * Indicates if the node is selected.
   */
  selected?: boolean;

  /**
   * The artifact of the node if it is a leaf.
   */
  artifact?: Artifact;

  /**
   * The source of the selection.
   */
  source?: SelectedArtifactSource;

  /**
   * Whether this is a summary node, which displays the summary in the side panel.
   */
  isSummary?: boolean;
}
