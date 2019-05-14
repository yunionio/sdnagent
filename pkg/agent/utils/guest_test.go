// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"io/ioutil"
	"path"
	"testing"
)

func TestGuestLoadDesc(t *testing.T) {
	serversPath := "/opt/cloud/workspace/servers/"
	fis, err := ioutil.ReadDir(serversPath)
	if err != nil {
		t.Skipf("scan servers path %s failed: %s", serversPath, err)
		return
	}
	for _, fi := range fis {
		if !fi.IsDir() {
			continue
		}
		id := fi.Name()
		path := path.Join(serversPath, id)
		g := &Guest{
			Id:   id,
			Path: path,
		}
		err := g.LoadDesc()
		if err != nil {
			t.Errorf("%s: load desc failed: %s", id, err)
		}
		t.Logf("%s: running: %v", id, g.Running())
	}
}
