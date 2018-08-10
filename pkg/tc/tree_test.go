package tc

import (
	"testing"
)

func TestQdiscTree(t *testing.T) {
	// output from tc qdisc show
	qstrs := []string{
		"qdisc tbf handle 1: root rate 500000Kbit burst 64000b latency 100ms mpu 64b",
		"qdisc fq_codel handle 10: parent 1:",
	}
	qs := []IQdisc{}
	for _, qstr := range qstrs {
		q, err := NewQdiscFromString(qstr)
		if err != nil {
			t.Fatalf("parse error: %s", err)
		}
		qs = append(qs, q)
	}
	qt, err := NewQdiscTree(qs)
	if err != nil {
		t.Fatalf("create qdisc tree error: %s", err)
	}
	// input for tc qdisc -batch
	batchReplaceLines := []string{
		"qdisc replace dev xxx root handle 1: tbf rate 500Mbit burst 64000b latency 100ms mpu 64b",
		"qdisc replace dev xxx parent 1: handle 10: fq_codel",
	}
	for i, line := range qt.BatchReplaceLines("xxx") {
		if line != batchReplaceLines[i] {
			t.Errorf("batch line %d:", i+1)
			t.Errorf("  want: %s", batchReplaceLines[i])
			t.Errorf("   got: %s", line)
		}
	}
}
