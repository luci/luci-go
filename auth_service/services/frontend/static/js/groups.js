// Copyright 2021 The LUCI Authors.
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

// GroupChooser is a scrollable list containing the auth service groups.
class GroupChooser {

  constructor(element) {
    // Root DOM element.
    this.element = document.getElementById(element);
  }

  // Loads list of groups from a server.
  // Updates group chooser UI. Returns deferred.
  refetchGroups() {
    var self = this;
    var defer = api.groups();
    defer
      .then((response) => {
        self.setGroupList(response.groups);
      })
      .catch((err) => {
        console.log(err);
      });
    return defer;
  }

  // Sets groupList (group-chooser) element.
  setGroupList(groups) {
    // Adds list item to group-chooser.
    const addElement = (group) => {
      if ('content' in document.createElement('template')) {
        var template = document.querySelector('#group-scroller-row-template');

        // Clone and grab elements to modify.
        var clone = template.content.cloneNode(true);
        var listEl = clone.querySelector('li');
        var name = clone.querySelector('p');
        var description = clone.querySelector('small');

        // Modify contents and append to parent.
        listEl.setAttribute('data-group-name', group.name);
        name.textContent = group.name;
        description.textContent = trimGroupDescription(group.description);
        this.element.appendChild(clone);
      } else {
        // TODO(cjacomet): Find another way to add group-chooser items.
        // HTML template element is not supported.
        console.error('Unable to load HTML template element, not supported.')
      }
      listEl.addEventListener('click', () => {
        this.setSelection(group.name);
      });
    };

    groups.map((group) => {
      addElement(group);
    });
  }

  // Adds the active class to the selected element,
  // highlighting the group clicked in the scroller.
  setSelection(name) {
    const groupElements = Array.from(document.getElementsByClassName('list-group-item'));

    groupElements.forEach((currentGroup) => {
      if (currentGroup.dataset.groupName === name) {
        currentGroup.classList.add('active');
      } else {
        currentGroup.classList.remove('active');
      }
    });
  }

}

// Trims group description to fit single line.
const trimGroupDescription = (desc) => {
  'use strict';
  if (desc == null) {
    return '';
  }
  var firstLine = desc.split('\n')[0];
  if (firstLine.length > 55) {
    firstLine = firstLine.slice(0, 55) + '...';
  }
  return firstLine;
}

window.onload = () => {
  // Setup global UI elements.
  var groupChooser = new GroupChooser('group-chooser');
  groupChooser.refetchGroups();
};