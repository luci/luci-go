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

import { fireEvent, render, screen } from '@testing-library/react';
import { expect } from 'chai';

import {
  Build,
  BuildAgentInputDataRef,
  BuildAgentOutput,
  BuildAgentResolvedDataRef,
  BuildInfra,
  BuildInfraBuildbucket,
} from '../../services/buildbucket';
import { BuildPackagesInfo } from './build_packages_info';

describe('BuildPackagesInfo', () => {
  it('build without resolved packages', async () => {
    const buildWithoutOutput = {
      input: { experiments: ['luci.buildbucket.agent.cipd_installation'] },
      infra: {
        buildbucket: {
          agent: {
            input: {
              data: {
                '': {
                  cipd: { specs: [{ package: 'input-pkg', version: 'input-ver' }] },
                } as BuildAgentInputDataRef,
              },
            },
          },
        } as Partial<BuildInfraBuildbucket> as BuildInfraBuildbucket,
      } as Partial<BuildInfra> as BuildInfra,
    } as Partial<Build> as Build;

    render(<BuildPackagesInfo build={buildWithoutOutput} />);
    expect(screen.getByText<HTMLButtonElement>('Resolved').disabled).to.be.true;
    expect(screen.getByText('Requested').ariaPressed).to.not.eq('true');
    expect(screen.queryByText('input-pkg')).to.be.null;

    // Display requested.
    fireEvent.click(screen.getByText('Requested'));
    expect(screen.getByText('Requested').ariaPressed).to.eq('true');
    expect(screen.queryByText('input-pkg')).to.not.be.null;

    // Click resolved, but it's disabled.
    fireEvent.click(screen.getByText('Resolved'));
    expect(screen.getByText('Requested').ariaPressed).to.eq('true');
    expect(screen.queryByText('input-pkg')).to.not.be.null;

    // Hide requested.
    fireEvent.click(screen.getByText('Requested'));
    expect(screen.getByText('Requested').ariaPressed).to.not.eq('true');
    expect(screen.queryByText('input-pkg')).to.be.null;
  });

  it('build with resolved packages', async () => {
    const buildWithoutOutput = {
      input: { experiments: ['luci.buildbucket.agent.cipd_installation'] },
      infra: {
        buildbucket: {
          agent: {
            input: {
              data: {
                '': {
                  cipd: { specs: [{ package: 'input-pkg', version: 'input-ver' }] },
                } as BuildAgentInputDataRef,
              },
            },
            output: {
              resolvedData: {
                '': {
                  cipd: { specs: [{ package: 'output-pkg', version: 'output-ver' }] },
                } as BuildAgentResolvedDataRef,
              },
            } as Partial<BuildAgentOutput> as BuildAgentOutput,
          },
        } as Partial<BuildInfraBuildbucket> as BuildInfraBuildbucket,
      } as Partial<BuildInfra> as BuildInfra,
    } as Partial<Build> as Build;

    render(<BuildPackagesInfo build={buildWithoutOutput} />);
    expect(screen.getByText<HTMLButtonElement>('Resolved').disabled).to.be.false;
    expect(screen.getByText('Requested').ariaPressed).to.not.eq('true');
    expect(screen.getByText('Resolved').ariaPressed).to.not.eq('true');
    expect(screen.queryByText('input-pkg')).to.be.null;
    expect(screen.queryByText('output-pkg')).to.be.null;

    // Display requested.
    fireEvent.click(screen.getByText('Requested'));
    expect(screen.getByText('Requested').ariaPressed).to.eq('true');
    expect(screen.getByText('Resolved').ariaPressed).to.not.eq('true');
    expect(screen.queryByText('input-pkg')).to.not.be.null;
    expect(screen.queryByText('output-pkg')).to.be.null;

    // Display resolved.
    fireEvent.click(screen.getByText('Resolved'));
    expect(screen.getByText('Resolved').ariaPressed).to.eq('true');
    expect(screen.getByText('Requested').ariaPressed).to.not.eq('true');
    expect(screen.queryByText('input-pkg')).to.be.null;
    expect(screen.queryByText('output-pkg')).to.not.be.null;

    // Hide resolved.
    fireEvent.click(screen.getByText('Resolved'));
    expect(screen.getByText('Requested').ariaPressed).to.not.eq('true');
    expect(screen.getByText('Resolved').ariaPressed).to.not.eq('true');
    expect(screen.queryByText('input-pkg')).to.be.null;
    expect(screen.queryByText('output-pkg')).to.be.null;
  });
});
