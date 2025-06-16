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

import { Chip } from '@mui/material';

import { getStatusStyle, StatusStyle } from '@/common/styles/status_styles';
import { StatusIcon } from '@/test_investigation/components/status_icon';
import { RateBoxDisplayInfo } from '@/test_investigation/utils/test_info_utils';

interface RateBoxProps {
  info: RateBoxDisplayInfo;
}

export function RateBox({ info }: RateBoxProps) {
  const style: StatusStyle = getStatusStyle(info.statusType);

  return (
    <>
      {style.icon && (
        <Chip
          label={info.text}
          sx={{
            backgroundColor: style.backgroundColor,
            borderColor: style.borderColor,
            borderWidth: style.borderColor ? '1px' : '0',
            borderStyle: style.borderColor ? 'solid' : 'none',
            pl: 1,
          }}
          icon={
            <StatusIcon
              iconType={style.icon}
              sx={{ fontSize: 16, color: style.iconColor || 'inherit' }}
            />
          }
        ></Chip>
      )}
    </>
  );
}
