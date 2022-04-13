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

class ChangeLogContent {
  constructor(element) {
    this.element = document.getElementById(element);
  }

  // Loads list of change logs from a server.
  // Updates change log list UI. Returns deferred.
  refetchChangeLogs() {
    var self = this;
    var defer = api.changeLogs();
    defer
      .then((response) => {
        self.setChangeLogList(response.changes);
      })
      .catch((err) => {
        console.log(err);
      });
    return defer;
  }

  setChangeLogList(logs) {
    const addElement = (log) => {
      if ('content' in document.createElement('template')) {
        var template = document.querySelector('#change-log-row-template')

        // Clone and grab elements to modify.
        var clone = template.content.cloneNode(true);
        var rev = clone.querySelector('td.change-log-rev');
        var type = clone.querySelector('td.change-log-type');
        var when = clone.querySelector('td.change-log-when');
        var who = clone.querySelector('td.change-log-who');

        // Modify contents and append to parent.
        rev.textContent = 'r' + log.authDbRev;
        type.textContent = log.changeType;
        when.textContent = common.utcTimestampToString(log.when);
        who.textContent = common.stripPrefix('user', log.who);
        // TODO: modify and append target's content.

        this.element.appendChild(clone);
      } else {
        // TODO: Find another way to add changeLogContent because the
        // HTML template element is not supported.
        console.error('Unable to load HTML template element, not supported.')
      }
    }

    logs.map((log) => {
      addElement(log);
    })
  }
}

window.onload = () => {
  var changeLogContent = new ChangeLogContent('change-log-content')
  changeLogContent.refetchChangeLogs();
}