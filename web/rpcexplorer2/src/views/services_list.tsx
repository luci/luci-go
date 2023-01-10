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

import { Link } from 'react-router-dom';

import { useGlobals } from '../context/globals';

const ServicesList = () => {
  const { descriptors } = useGlobals();

  return (
    <>
      <p>List of services:</p>
      <ul>
        {descriptors.services.map((svc) => {
          return (
            <li key={svc.name}>
              <Link to={svc.name}>{svc.name}</Link>{svc.help}
            </li>
          );
        })}
      </ul>
    </>
  );
};

export default ServicesList;
