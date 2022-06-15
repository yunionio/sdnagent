package server

import (
	"testing"

	"yunion.io/x/jsonutils"
)

// ovs-vsctl --format=json --columns=name,mirrors list Bridge
func TestFetchMirrorIdBridgeMapInternal(t *testing.T) {
	for _, json := range []string{
		`{"data":[["br1",["set",[]]],["br0",["set",[["uuid","5ab854d3-b050-48de-9d60-3f5791478d1c"],["uuid","d5dfa2a6-7633-4f13-89d9-ecfa2b161bda"]]]],["brtap",["set",[]]],["brmapped",["set",[]]],["breip",["set",[]]],["brvpc",["set",[]]]],"headings":["name","mirrors"]}`,
		`{"data":[["br1",["set",[]]],["br0",["set",[]]],["brtap",["set",[]]],["brmapped",["set",[]]],["breip",["set",[]]],["brvpc",["set",[]]]],"headings":["name","mirrors"]}`,
		`{"data":[["br1",["set",[]]],["br0",["uuid","518561f0-2b69-46c3-9455-dd04d01dc5f5"]],["brtap",["set",[]]],["brmapped",["set",[]]],["breip",["set",[]]],["brvpc",["set",[]]]],"headings":["name","mirrors"]}`,
	} {
		ret, err := fetchMirrorIdBridgeMapInternal([]byte(json))
		if err != nil {
			t.Errorf("fetchMirrorIdBridgeMapInternal fail %s", err)
		} else {
			t.Logf("%s", jsonutils.Marshal(ret))
		}
	}
}

// ovs-vsctl --format=json --columns=name,_uuid,output_port list Mirror
func TestFetchMirrorNameIdMapInternal(t *testing.T) {
	for _, json := range []string{
		`{"data":[["m0018",["uuid","d5dfa2a6-7633-4f13-89d9-ecfa2b161bda"],["uuid","62208f49-cf74-4275-8db9-34a023a686c9"]],["m0017",["uuid","5ab854d3-b050-48de-9d60-3f5791478d1c"],["set",[]]]],"headings":["name","_uuid","output_port"]}`,
		`{"data":[["m0018",["uuid","518561f0-2b69-46c3-9455-dd04d01dc5f5"],["uuid","62208f49-cf74-4275-8db9-34a023a686c9"]]],"headings":["name","_uuid","output_port"]}`,
	} {
		ret, err := fetchMirrorNameIdMapInternal([]byte(json))
		if err != nil {
			t.Errorf("fetchMirrorNameIdMapInternal fail %s", err)
		} else {
			t.Logf("%s", jsonutils.Marshal(ret))
		}
	}
}
