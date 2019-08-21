package utils

import (
	"testing"
)

var routesRaw string = `
Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT                                                       
br0	00000000	01DEA80A	0003	0	0	0	00000000	0	0	0                                                                                
veth-gwa	0002000A	00000000	0001	0	0	0	00FFFFFF	0	0	0                                                                           
br0	00DEA80A	00000000	0001	0	0	0	00FFFFFF	0	0	0                                                                                
br0	FEA9FEA9	01DEA80A	0007	0	0	0	FFFFFFFF	0	0	0                                                                                
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0                                                                            
br-lan	0001A8C0	00000000	0001	0	0	0	00FFFFFF	0	0	0                                                                             
br-wan	0007A8C0	00000000	0001	0	0	0	00FFFFFF	0	0	0                                                                             
`

func TestRoutes(t *testing.T) {
	routes, err := parseRoutes(routesRaw)
	if err != nil {
		t.Fatalf("parse routes: %v", err)
	}
	t.Logf("routes:\n%s", routes.String())

	t.Run("route lookup", func(t *testing.T) {
		route, err := routes.Lookup("10.0.2.198")
		if err != nil {
			t.Fatalf("lookup failed: %v", err)
		}
		if route.Dev != "veth-gwa" {
			t.Errorf("unexpected lookup result: %s", route.String())
		}
	})
}
