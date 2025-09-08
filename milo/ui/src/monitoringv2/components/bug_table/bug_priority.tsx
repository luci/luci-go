import { Box, SxProps } from '@mui/material';

export interface BugPriorityProps {
  priority: number | undefined;
}
export const BugPriority = ({ priority }: BugPriorityProps) => {
  if (priority === undefined) {
    return null;
  }
  let style: SxProps = {
    width: '26px',
    height: '24px',
    textAlign: 'center',
    lineHeight: '20px',
    padding: '1px 2px',
    boxSizing: 'border-box',
    borderRadius: '4px',
    border: 'solid 2px #fff',
  };
  if (priority === 0) {
    style = {
      ...style,
      backgroundColor: '#1a73e8',
      border: 'solid 2px #1a73e8',
      color: '#fff',
    };
  }
  if (priority === 1) {
    style = { ...style, border: 'solid 2px #1a73e8', borderRadius: '4px' };
  }
  return <Box sx={style}>P{priority}</Box>;
};
