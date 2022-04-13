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
    const parseTarget = (t) => {
      var kind = t.split('$', 1)[0];
      var name = t.substring(kind.length + 1);

      // Recognize some known targets.
      var title = name;
      var targetURL = null;
      switch (kind) {
        case 'AuthGroup':
          targetURL = '/groups/' + name;
          break;
        case 'AuthIPWhitelist':
          targetURL = '/ip_allowlists';
          break;
        case 'AuthIPWhitelistAssignments':
          title = 'IP allowlist assignment';
          targetURL = '/ip_allowlists';
          break;
        case 'AuthGlobalConfig':
          title = 'Global config';
          targetURL = '/oauth_config';
          break;
        case 'AuthRealmsGlobals':
          title = 'Realms config';
          break;
      }

      return {
        kind: kind,
        name: name,
        title: title,
        changeLogTargetURL: common.getChangeLogTargetURL(kind, name),
        targetURL: targetURL
      };
    }

    const addElement = (log) => {
      if ('content' in document.createElement('template')) {
        var template = document.querySelector('#change-log-row-template')

        // Clone and grab elements to modify.
        var clone = template.content.cloneNode(true);
        var rev = clone.querySelector('td.change-log-rev a');
        var type = clone.querySelector('td.change-log-type');
        var when = clone.querySelector('td.change-log-when');
        var who = clone.querySelector('td.change-log-who');
        var target = clone.querySelector('td.change-log-target a');

        // Modify contents and append to parent.
        var t = parseTarget(log.target);
        rev.href = common.getChangeLogRevisionURL(log.authDbRev);
        rev.textContent = 'r' + log.authDbRev;
        type.textContent = log.changeType;
        when.textContent = common.utcTimestampToString(log.when);
        who.textContent = common.stripPrefix('user', log.who);
        target.href = t.changeLogTargetURL;
        target.textContent = t.title;

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