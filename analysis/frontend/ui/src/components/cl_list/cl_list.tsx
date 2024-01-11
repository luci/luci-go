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

import {
  useState,
  Fragment,
  MouseEvent,
} from 'react';

import { styled } from '@mui/material/styles';
import ClickAwayListener from '@mui/material/ClickAwayListener';
import Link from '@mui/material/Link';
import Tooltip, { TooltipProps, tooltipClasses } from '@mui/material/Tooltip';

import {
  clLink,
} from '@/tools/urlHandling/links';
import {
  Changelist,
} from '@/legacy_services/shared_models';

interface Props {
  changelists: Changelist[];
}

const LightTooltip = styled(({ className, ...props }: TooltipProps) => (
  <Tooltip {...props} classes={{ popper: className }} />
))(({ theme }) => ({
  [`& .${tooltipClasses.arrow}`]: {
    color: theme.palette.common.white,
  },
  [`& .${tooltipClasses.tooltip}`]: {
    backgroundColor: theme.palette.common.white,
    color: 'inherit',
    boxShadow: theme.shadows[1],
    fontSize: theme.typography.fontSize,
    fontWeight: theme.typography.fontWeightRegular,
  },
}));

const CLList = ({
  changelists,
}: Props) => {
  const [tooltipOpen, setTooltipOpen] = useState(false);

  const handleTooltipClose = () => {
    setTooltipOpen(false);
  };

  const handleTooltipOpen = (e: MouseEvent<HTMLAnchorElement>) => {
    setTooltipOpen(!tooltipOpen);
    e.preventDefault();
  };


  if (changelists.length == 0) {
    return <></>;
  }
  return (
    <ClickAwayListener onClickAway={handleTooltipClose}>
      <div style={{ display: 'inline-block' }}>
        <Link
          href={clLink(changelists[0])}
          target="_blank"
        >
          {changelists[0].change}&nbsp;#{changelists[0].patchset}
        </Link>
        {changelists.length > 1 ? (
          <>
            {', '}
            <LightTooltip
              PopperProps={{
                disablePortal: true,
              }}
              open={tooltipOpen}
              onClose={handleTooltipClose}
              disableFocusListener
              disableHoverListener
              disableTouchListener
              arrow
              title={
                <>
                  {
                    changelists.slice(1).map((cl, i) => {
                      return (
                        <Fragment key={i.toString()}>
                          {i > 0 ? ', ' : null}
                          <Link
                            href={clLink(cl)}
                            target="_blank"
                          >
                            {cl.change}&nbsp;#{cl.patchset}
                          </Link>
                        </Fragment>
                      );
                    })
                  }
                </>
              }>
              <Link href="#" onClick={handleTooltipOpen}>...</Link>
            </LightTooltip>
          </>) : null
        }
      </div>
    </ClickAwayListener>
  );
};

export default CLList;
