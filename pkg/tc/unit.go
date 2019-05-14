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

package tc

import (
	"fmt"
	"strconv"
	"strings"
)

const TIME_UNIT_PER_SECOND uint64 = 1000000

var rateSuffixes = map[string]uint64{
	"bit":   1,
	"Kibit": 1024,
	"kbit":  1000,
	"mibit": 1024 * 1024,
	"mbit":  1000000,
	"gibit": 1024 * 1024 * 1024,
	"gbit":  1000000000,
	"tibit": 1024 * 1024 * 1024 * 1024,
	"tbit":  1000000000000,
	"Bps":   8,
	"KiBps": 8 * 1024,
	"KBps":  8000,
	"MiBps": 8 * 1024 * 1024,
	"MBps":  8000000,
	"GiBps": 8 * 1024 * 1024 * 1024,
	"GBps":  8000000000,
	"TiBps": 8 * 1024 * 1024 * 1024 * 1024,
	"TBps":  8000000000000,
}

func init() {
	// make later lookups case-insensitive
	for suffix, scale := range rateSuffixes {
		lower := strings.ToLower(suffix)
		if suffix != lower {
			rateSuffixes[lower] = scale
			delete(rateSuffixes, suffix)
		}
	}
}

func parseNumSuffix(s string) (num float64, suffix string, err error) {
	if len(s) == 0 {
		err = fmt.Errorf("zero length rate string")
		return
	}
	i := 0
	for ; i < len(s); i++ {
		if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
			break
		}
	}
	if i == len(s) {
		err = fmt.Errorf("missing unit suffix")
		return
	}
	num, err = strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return
	}
	return num, s[i:], nil
}

func ParseRate(s string) (bytesPerSec uint64, err error) {
	rate, suffix, err := parseNumSuffix(s)
	if err != nil {
		return
	}
	scale, ok := rateSuffixes[strings.ToLower(suffix)]
	if !ok {
		err = fmt.Errorf("unknown suffix %s", suffix)
		return
	}
	bytesPerSec = uint64(rate) * scale / 8
	return
}

// TODO rename to SprintfXXX
func PrintRate(bytesPerSec uint64) string {
	UNITS := [5]string{"", "K", "M", "G", "T"}
	var i int

	bps := bytesPerSec * 8
	for i, _ = range UNITS {
		if bps < 1000 {
			break
		}
		if (bps%1000 != 0) && bps < 1000*1000 {
			break
		}
		if i+1 == len(UNITS) {
			break
		}
		bps /= 1000
	}
	rate := fmt.Sprintf("%d%sbit", bps, UNITS[i])
	return rate
}

func ParseTime(s string) (us uint64, err error) {
	num, suffix, err := parseNumSuffix(s)
	if err != nil {
		return
	}
	us = uint64(num)
	switch strings.ToLower(suffix) {
	case "s", "sec":
		us = us * TIME_UNIT_PER_SECOND
	case "ms", "msec", "msecs":
		us = us * TIME_UNIT_PER_SECOND / 1000
	case "us", "usec", "usecs":
		us = us * TIME_UNIT_PER_SECOND / 1000000
	default:
		err = fmt.Errorf("unknown time unit %s", suffix)
	}
	return
}

func PrintTime(us uint64) string {
	scales := []struct {
		nus  uint64
		unit string
	}{
		{nus: 1000 * 1000, unit: "s"},
		{nus: 1000, unit: "ms"},
		{nus: 1, unit: "us"},
	}
	var i int
	for i, _ = range scales {
		if us >= scales[i].nus {
			break
		}
	}
	var v uint64
	scale := scales[i]
	if i == len(scales)-1 {
		v = us
	} else if (us%(scale.nus) != 0) && us < scale.nus*1000 {
		scale = scales[i+1]
		v = us / scale.nus
	} else {
		v = us / scale.nus
	}
	return fmt.Sprintf("%d%s", v, scale.unit)
}

func ParseSize(s string) (bytes uint64, err error) {
	num, suffix, err := parseNumSuffix(s)
	if err != nil {
		return
	}
	bytes = uint64(num)
	switch strings.ToLower(suffix) {
	case "b", "":
	case "kb", "k":
		bytes *= 1024
	case "mb", "m":
		bytes *= 1024 * 1024
	case "gb", "g":
		bytes *= 1024 * 1024 * 1024
	case "kbit":
		bytes *= 1024 / 8
	case "mbit":
		bytes *= 1024 * 1024 / 8
	case "gbit":
		bytes *= 1024 * 1024 * 1024 / 8
	default:
		err = fmt.Errorf("unknown size suffix %s", suffix)
	}
	return
}

func PrintSize(bytes uint64) string {
	if bytes >= 1024*1024 && bytes-(1024*1024)*(bytes/(1024*1024)) < 1024 {
		return fmt.Sprintf("%dMb", bytes/(1024*1024))
	} else if bytes >= 1024 && bytes-(1024)*(bytes/(1024)) < 16 {
		return fmt.Sprintf("%dKb", bytes/(1024))
	} else {
		return fmt.Sprintf("%db", bytes)
	}
}

func TcTbfBurstNormalize(bytesPerSec uint64, burst uint64) uint64 {
	// tc parses burst into buffer
	time := uint64(1000000 * float64(burst) / float64(bytesPerSec))
	buffer := uint64(tickInUsec * float64(time)) // aka. ticks
	// then translates it back on query
	burst = bytesPerSec * uint64(float64(buffer)/tickInUsec) / 1000000
	return burst
}
