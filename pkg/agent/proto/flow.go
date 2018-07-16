package pb

import (
	"fmt"
	"strings"

	"github.com/digitalocean/go-openvswitch/ovs"
)

func (f *Flow) OvsFlow() (*ovs.Flow, error) {
	chunks := []string{}
	if f.Cookie != 0 {
		chunks = append(chunks, fmt.Sprintf("cookie=0x%x", f.Cookie))
	}
	if f.Table != 0 {
		chunks = append(chunks, fmt.Sprintf("table=%d", f.Table))
	}
	chunks = append(chunks, fmt.Sprintf("priority=%d", f.Priority))
	chunks = append(chunks, f.Matches)
	chunks = append(chunks, fmt.Sprintf("actions=%s", f.Actions))
	txt := strings.Join(chunks, ",")
	of := &ovs.Flow{}
	err := of.UnmarshalText([]byte(txt))
	if err != nil {
		return nil, err
	}
	return of, nil
}
