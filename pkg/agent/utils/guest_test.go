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
