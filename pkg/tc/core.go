package tc

import (
	"fmt"
	"os"
)

// how many ticks within one microsecond
var tickInUsec float64 = float64(0x3e8) / float64(0x40)

func init() {
	f, err := os.Open("/proc/net/psched")
	if err != nil {
		return
	}
	var t2us, us2t, clockRes, bufferHz uint32
	n, err := fmt.Fscanf(f, "%x %x %x %x", &t2us, &us2t, &clockRes, &bufferHz)
	if n != 4 || err != nil {
		return
	}
	tickInUsec = float64(t2us) / float64(us2t)
}
