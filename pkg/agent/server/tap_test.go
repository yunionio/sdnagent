package server

import (
	"testing"
)

func TestFetchMirrorIdBridgeMapInternal(t *testing.T) {
	json := `{"data":[["brmapped",["set",[]]],["brvpc",["uuid","54df328a-a6ce-4576-a978-407d6cad2944"]],["br0",["set",[]]],["brtap",["set",[]]]],"headings":["name","mirrors"]}`
	ret, err := fetchMirrorIdBridgeMapInternal([]byte(json))
	if err != nil {
		t.Errorf("fetchMirrorIdBridgeMapInternal fail %s", err)
	} else {
		t.Logf("%s", ret)
	}
}

func TestFetchMirrorNameIdMapInternal(t *testing.T) {
	json := `{"data":[["m0014",["uuid","54df328a-a6ce-4576-a978-407d6cad2944"]]],"headings":["name","_uuid"]}`
	ret, err := fetchMirrorNameIdMapInternal([]byte(json))
	if err != nil {
		t.Errorf("fetchMirrorNameIdMapInternal fail %s", err)
	} else {
		t.Logf("%s", ret)
	}
}
