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
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import ClearIcon from '@mui/icons-material/Clear';
import AddCircleIcon from '@mui/icons-material/AddCircle';
import DoneIcon from '@mui/icons-material/Done';
import Checkbox from '@mui/material/Checkbox';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import {useState, forwardRef, useImperativeHandle, useEffect} from 'react';
import { isGlob, isMember, isSubgroup } from '@/authdb/common/helpers';

import './groups_list.css';

interface GroupsFormListProps {
    // Sets the starting items array. Used on initial GetGroup call from groups_form.
    initialItems: string[];
    // This will be either members, subgroups or globs. Used for header in form and to check validity of added items.
    name: string;
    // groups_form function called when items are added or removed in this array.
    // Also called when items are 'reset' (same as initial).
    // This allows groups_form component to know of edited state.
    itemsChanged: () => void;
}

export interface FormListElement {
  getItems: () => string[];
  setReadonly: () => void;
  changeItems: (items: string[]) => void;
  isChanged: () => boolean;
}

type Item = {
  value: string;
  include: boolean;
};

export const GroupsFormList = forwardRef<FormListElement, GroupsFormListProps>(
  (
  {initialItems, name, itemsChanged}, ref
  ) => {
    const [addingItem, setAddingItem] = useState<boolean>();
    const [currentItem, setCurrentItem] = useState<string>();
    const [errorMessage, setErrorMessage] = useState<string>('');
    let initial:Item[] = [];
    initialItems.forEach((item) => {
      initial.push({
        value: item,
        include: true,
      })
    })
    const [items, setItems] = useState<Item[]>(initial);

    useImperativeHandle(ref, () => ({
      getItems: () => {
        return getCurrentItemList();
      },
      setReadonly: () => {
        setAddingItem(false);
        setCurrentItem("");
      },
      changeItems: (items: string[]) => {
        let newItems:Item[] = [];
        items.forEach((item) => {
          newItems.push({
            value: item,
            include: true,
          })
        })
        setItems(newItems);
      },
      isChanged: () => {
        return !(items.length === initialItems.length && items.every((val, index) => val.value === initialItems[index]) && items.every((item) => item.include === true));
      }
    }));

    const resetTextfield = () => {
        setAddingItem(!addingItem);
        setCurrentItem('');
        setErrorMessage('');
      }

    // Returns item list with removed items taken away to be sent for update.
    const getCurrentItemList = () => {
      let updatedItems: string[] = []
      items.forEach((item) => {
        if (item.include) {
          updatedItems.push(item.value);
        }
      })
      return updatedItems;
    }

    const addToItems = () => {
      // Make sure item added is not a duplicate.
      for (const item of items) {
        if (item.value == currentItem!) {
          setErrorMessage('Duplicate item.');
          return;
        }
      }
      // If this is members or globs, verify accordingly before adding.
      // If it doesn't meet the requirements, show error message.
      if (name == 'Members') {
        if (!isMember(currentItem!)) {
          setErrorMessage('Each member should be an email address.');
          return;
        }
      } else if (name == 'Globs') {
        if (!isGlob(currentItem!)) {
          setErrorMessage('Each glob should use at least one wildcard (i.e. *).');
          return;
        }
      } else if (name == 'Subgroups') {
        if (!isSubgroup(currentItem!)) {
          setErrorMessage('Invalid subgroup name.');
          return;
        }
      }
      if (currentItem) {
        setItems([...items, {value: currentItem, include: true}]);
        resetTextfield()
      }
    }

    useEffect(() => {
      itemsChanged();
    }, [items])

    const isNewItem = (item: string) => {
      if (initialItems.includes(item)) {
        return false;
      }
      return true;
    }

    const submitItem = (e: React.KeyboardEvent<HTMLDivElement>) => {
        if (e.key == 'Enter') {
          addToItems();
        }
      }

    const handleChange = (index: number) => {
      const updatedItems = [...items];
      // This is a new item, so just remove.
      if (index >= initialItems.length) {
        updatedItems.splice(index, 1)
      } else {
        updatedItems[index].include = !updatedItems[index].include
      }
      setItems(updatedItems);
    }

    return (
    <TableContainer data-testid='groups-form-list'>
    <Table sx={{ p: 0, pt: '15px', width: '100%' }} data-testid='mouse-enter-table'>
        <TableBody>
      <TableRow>
        <TableCell colSpan={2} sx={{pb :0}} style={{display: 'flex', flexDirection: 'row', alignItems:'center', minHeight: '45px'}}>
          <Typography variant="h6"> {name}</Typography>
        </TableCell>
      </TableRow>
      {items && items.map((item, index) =>
        <TableRow key={index} style={{height: '34px'}} sx={{borderBottom: '1px solid grey'}} className='item-row' data-testid={`item-row-${item.value}`}>
          <TableCell sx={{p: 0, pt: '1px'}} style={{display: 'flex', flexDirection: 'row', alignItems:'center', minHeight: '30px'}}>
            <Checkbox sx={{pt: 0, pb: 0}} checked={item.include} data-testid={`checkbox-button-${item.value}`} id={`${index}`} onChange={() => {handleChange(index)}}/>
            {isNewItem(item.value)
            ? <Typography variant="body2" color="green">{item.value}</Typography>
            : <>
              {(!item.include)
                ? <Typography variant="body2" color="red" style={{textDecoration:'line-through'}} data-testid={`removed-item-${item.value}`} id="removed-item">{item.value}</Typography>
                : <Typography variant="body2">{item.value}</Typography>
              }
              </>
            }
          </TableCell>
        </TableRow>
      )}
      {addingItem && (
        <TableRow>
          <TableCell sx={{p: 0, pt: '15px', pr: '15px'}} style={{width: '94%'}}>
            <TextField label='Add New' style={{width: '100%'}} onChange={(e) => setCurrentItem(e.target.value)} onKeyDown={submitItem} value={currentItem} data-testid='add-textfield' error={errorMessage !== ''} helperText={errorMessage}></TextField>
          </TableCell>
          <TableCell align='center' style={{width: '3%'}} sx={{p: 0, pt: '15px'}}>
            <IconButton color='success' sx={{p: 0}} onClick={() => {addToItems()}} data-testid='confirm-button'>
              <DoneIcon />
            </IconButton>
          </TableCell>
          <TableCell align='center' style={{width: '3%'}} sx={{p: 0, pt: '15px'}}>
            <IconButton color='error' sx={{p: 0}} onClick={resetTextfield} data-testid='clear-button'>
              <ClearIcon />
            </IconButton>
          </TableCell>
        </TableRow>)
      }
      {!addingItem &&
        <TableRow>
            <TableCell>
          <Button sx={{mt: '10px'}} variant="outlined" startIcon={<AddCircleIcon />} onClick={resetTextfield} data-testid='add-button'>
            Add
          </Button>
          </TableCell>
      </TableRow>
      }
      </TableBody>
    </Table>
  </TableContainer>);
}
);