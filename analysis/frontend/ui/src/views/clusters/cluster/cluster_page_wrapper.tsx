// Copyright 2022 The LUCI Authors.
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

import './cluster_page';
import '../../../../web_component_types';

import { useCallback } from 'react';
import {
  useNavigate,
  useParams,
} from 'react-router-dom';

const ClusterPageWrapper = () => {
  const { project, algorithm, id } = useParams();
  const navigate = useNavigate();
  const elementRef = useCallback((node) => {
    if (node !== null) {
      node.navigate = navigate;
    }
  }, [navigate]);
  return (
    <cluster-page
      ref={elementRef}
      project={project}
      clusterAlgorithm={algorithm || 'rules-v2'}
      clusterId={id}
    />
  );
};

export default ClusterPageWrapper;
