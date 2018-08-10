package tc

import (
	"testing"
)

func TestParseRate(t *testing.T) {
	kiloK := uint64(1000)
	kiloM := kiloK * 1000
	kiloG := kiloM * 1000
	cases := []struct {
		s       string
		want    uint64
		wantp   string
		invalid bool
	}{
		{s: "100bit", want: 100 / 8, wantp: "96bit"},
		{s: "100Kbit", want: 100 * kiloK / 8, wantp: "100Kbit"},
		{s: "100Mbit", want: 100 * kiloM / 8, wantp: "100Mbit"},
		{s: "1001Mbit", want: 1001 * kiloM / 8, wantp: "1001Mbit"},
		{s: "1010Mbit", want: 1010 * kiloM / 8, wantp: "1010Mbit"},
		{s: "1100Mbit", want: 1100 * kiloM / 8, wantp: "1100Mbit"},
		{s: "1000Mbit", want: 1000 * kiloM / 8, wantp: "1Gbit"},
		{s: "1Gbit", want: kiloG / 8, wantp: "1Gbit"},
	}

	for _, c := range cases {
		bytesPerSec, err := ParseRate(c.s)
		if !c.invalid && err != nil {
			t.Errorf("!invalid, got %s", err)
			continue
		}
		if c.want != bytesPerSec {
			t.Errorf("parse: want %d, got %d", c.want, bytesPerSec)
			continue
		}
		if got := PrintRate(bytesPerSec); c.wantp != got {
			t.Errorf("print: want %s, got %s", c.wantp, got)
		}
	}
}

func TestParseTime(t *testing.T) {
	cases := []struct {
		s       string
		want    uint64
		wantp   string
		invalid bool
	}{
		{s: "100ms", want: 100 * 1000, wantp: "100ms"},
		{s: "1000ms", want: 1000 * 1000, wantp: "1s"},
		{s: "1001ms", want: 1001 * 1000, wantp: "1001ms"},
	}

	for _, c := range cases {
		us, err := ParseTime(c.s)
		if !c.invalid && err != nil {
			t.Errorf("!invalid, got %s", err)
			continue
		}
		if c.want != us {
			t.Errorf("parse: want %d, got %d", c.want, us)
			continue
		}
		if got := PrintTime(us); c.wantp != got {
			t.Errorf("print: want %s, got %s", c.wantp, got)
		}
	}
}

func TestTcTbfBurstNormalize(t *testing.T) {
	rate := uint64(10) * 1000 * 1000 * 1000 / 8
	burst := uint64(1220 * 1024)
	want := uint64(1247500)
	got := TcTbfBurstNormalize(rate, burst)
	if want != got {
		t.Errorf("TcTbfBurstNormalize(%q), want %d, got %d", burst, want, got)
	}
}
