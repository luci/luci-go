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

////////////////////////////////////////////////////////////////////////////////
// Selector is the dropdown selection menu containing allowlists.
class Selector {
  constructor(element) {
    // Root DOM element.
    this.element = document.getElementById(element);
  }

  reloadAllowlists() {
    let defer = api.ipAllowlists();
    defer
      .then((response) => {
        this.populate(response.allowlists);
      })
      .catch((err) => {
        console.log(err);
      });
    return defer;
  }

  populate(allowlists) {
    let addToSelector = (name) => {
      let option = document.createElement('option');
      option.text = name;
      this.element.appendChild(option);
    };

    // All allowlists
    allowlists.map((list) => {
      addToSelector(list.name);
    });
  }
}

window.onload = () => {
  let selector = new Selector('allowlist-selector');
  selector.reloadAllowlists();
};
