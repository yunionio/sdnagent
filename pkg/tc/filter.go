package tc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
)

/*
 * // add fw
 * tc filter add dev br0 protocol ip parent 1: prio 1 handle 599 fw classid 1:3
 * // show fw
 * filter parent 1: protocol ip pref 1 fw chain 0
 * filter parent 1: protocol ip pref 1 fw chain 0 handle 0x257 classid 1:3
 * // add u32
 * tc filter add dev vnet2202-232 parent ffff: protocol ip u32 match u32 0 0 action mirred egress redirect dev rvnet2202-232
 * // show u32
 * filter parent ffff: protocol ip pref 49152 u32 chain 0
 * filter parent ffff: protocol ip pref 49152 u32 chain 0 fh 800: ht divisor 1
 * filter parent ffff: protocol ip pref 49152 u32 chain 0 fh 800::800 order 2048 key ht 800 bkt 0 terminal flowid ??? not_in_hw
 *   match 00000000/00000000 at 0
 *   action order 1: mirred (Egress Redirect to device reth0) stolen
 *   index 1 ref 1 bind 1
 */

type IFilter interface {
	ITcObj

	ITcObjAlter

	Base() *SBaseTcFilter

	ParentQdisc() IQdisc
	Priority() uint32
}

type SBaseTcFilter struct {
	Parent   IQdisc
	Kind     string
	Protocol string
	Prio     uint32
}

func (f *SBaseTcFilter) Id() string {
	return ""
}

func (f *SBaseTcFilter) ParentQdisc() IQdisc {
	return f.Parent
}

func (f *SBaseTcFilter) Priority() uint32 {
	return f.Prio
}

func (f *SBaseTcFilter) Compare(fi IComparable) int {
	f2, ok := fi.(*SBaseTcFilter)
	if !ok {
		return -1
	}

	if f.Parent == nil && f2.Parent != nil {
		return -1
	}
	if f.Parent != nil && f2.Parent == nil {
		return 1
	}
	if f.Parent != nil && f2.Parent != nil && f.Parent.Id() != f2.Parent.Id() {
		return compareClassId(f.Parent.Id(), f2.Parent.Id())
	}
	if f.Prio < f2.Prio {
		return -1
	} else if f.Prio > f2.Prio {
		return 1
	}
	if f.Protocol != f2.Protocol {
		return strings.Compare(f.Protocol, f2.Protocol)
	}
	if f.Kind != f2.Kind {
		return strings.Compare(f.Kind, f2.Kind)
	}
	return 0
}

func (q *SBaseTcFilter) CompareBase(qi IComparable) int {
	return q.Compare(qi)
}

func (f *SBaseTcFilter) Equals(fi IComparable) bool {
	return f.Compare(fi) == 0
}

// tc filter replace dev eth0 parent 1: prio 1 handle 10: flower src_ip 192.168.1.10 action mirred egress redirect dev ifb0
func (f *SBaseTcFilter) basicLineElements(action string, ifname string) []string {
	elms := []string{"filter", action, "dev", ifname}
	if f.Parent != nil {
		elms = append(elms, "parent", f.Parent.Id())
	}
	if len(f.Protocol) > 0 {
		elms = append(elms, "protocol", f.Protocol)
	}
	elms = append(elms, "prio", fmt.Sprintf("%d", f.Prio))
	return elms
}

func parseBaseFilter(chunks []string, parents []IQdisc) (*SBaseTcFilter, error) {
	f := &SBaseTcFilter{}
	for i := 0; i < len(chunks); {
		c := chunks[i]
		switch c {
		case "parent":
			if i+1 >= len(chunks) {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "eol before getting parent")
			}
			parentId := chunks[i+1]
			for _, parent := range parents {
				if parent.Id() == parentId {
					f.Parent = parent
					break
				}
			}
			if f.Parent == nil {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "parent not found")
			}
			i += 2
		case "fw":
			f.Kind = "fw"
			i += 1
		case "u32":
			f.Kind = "u32"
			i += 1
		case "pref":
			if i+1 >= len(chunks) {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "eol before getting pref")
			}
			prefInt, err := strconv.ParseUint(chunks[i+1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid pref %s: %v", chunks[i+1], err)
			}
			f.Prio = uint32(prefInt)
			i += 2
		case "protocol":
			if i+1 >= len(chunks) {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "eol before getting protocol")
			}
			f.Protocol = chunks[i+1]
			i += 2
		default:
			i++
		}
	}
	return f, nil
}

type SFwFilter struct {
	*SBaseTcFilter
	ClassId string
	Handle  uint32
}

func (f *SFwFilter) Base() *SBaseTcFilter {
	return f.SBaseTcFilter
}

func (f *SFwFilter) Compare(itc IComparable) int {
	baseFilter, ok := itc.(IFilter)
	if !ok {
		return -1
	}
	baseCmp := f.Base().Compare(baseFilter.Base())
	if baseCmp != 0 {
		return baseCmp
	}
	f2 := baseFilter.(*SFwFilter)
	if f.ClassId != f2.ClassId {
		return strings.Compare(f.ClassId, f2.ClassId)
	}
	if f.Handle < f2.Handle {
		return -1
	} else if f.Handle > f2.Handle {
		return 1
	}
	return 0
}

func (f *SFwFilter) Equals(fi IComparable) bool {
	return f.Compare(fi) == 0
}

func (f *SFwFilter) basicLineElements(action string, ifname string) []string {
	elms := f.SBaseTcFilter.basicLineElements(action, ifname)
	elms = append(elms, "handle", fmt.Sprintf("0x%x", f.Handle))
	elms = append(elms, "fw")
	elms = append(elms, "classid", f.ClassId)
	return elms
}

func (f *SFwFilter) AddLine(ifname string) string {
	elms := f.basicLineElements("add", ifname)
	return strings.Join(elms, " ")
}

func (f *SFwFilter) ReplaceLine(ifname string) string {
	elms := f.basicLineElements("replace", ifname)
	return strings.Join(elms, " ")
}

func (f *SFwFilter) DeleteLine(ifname string) string {
	elms := f.basicLineElements("delete", ifname)
	return strings.Join(elms, " ")
}

func parseFwFilter(chunks []string) (*SFwFilter, error) {
	f := &SFwFilter{}
	for i := 0; i < len(chunks); {
		c := chunks[i]
		switch c {
		case "classid":
			i++
			if i >= len(chunks) {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "eol before getting classid")
			}
			f.ClassId = chunks[i]
		case "handle":
			i++
			if i >= len(chunks) {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "eol before getting handle")
			}
			numStr := chunks[i]
			base := 10
			if strings.HasPrefix(numStr, "0x") {
				base = 16
				numStr = numStr[2:]
			}
			handleInt, err := strconv.ParseUint(numStr, base, 64)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid handle %s", chunks[i])
			}
			f.Handle = uint32(handleInt)
		default:
			i++
		}
	}
	if len(f.ClassId) == 0 {
		return nil, errors.Wrap(errors.ErrInvalidFormat, "classid not found")
	}
	return f, nil
}

type SU32Filter struct {
	*SBaseTcFilter
	RedirectDev string
}

func (f *SU32Filter) Base() *SBaseTcFilter {
	return f.SBaseTcFilter
}

func (f *SU32Filter) Compare(itc IComparable) int {
	baseFilter, ok := itc.(IFilter)
	if !ok {
		return -1
	}
	baseCmp := f.Base().Compare(baseFilter.Base())
	if baseCmp != 0 {
		return baseCmp
	}
	f2 := baseFilter.(*SU32Filter)
	if f.RedirectDev != f2.RedirectDev {
		return strings.Compare(f.RedirectDev, f2.RedirectDev)
	}
	return 0
}

func (f *SU32Filter) Equals(fi IComparable) bool {
	return f.Compare(fi) == 0
}

func (f *SU32Filter) basicLineElements(action string, ifname string) []string {
	elms := f.SBaseTcFilter.basicLineElements(action, ifname)
	if len(f.RedirectDev) > 0 {
		elms = append(elms,
			"u32",
			"match",
			"u32",
			"0",
			"0",
			"action",
			"mirred",
			"egress",
			"redirect",
			"dev",
			f.RedirectDev,
		)
	}
	return elms
}

func (f *SU32Filter) AddLine(ifname string) string {
	elms := f.basicLineElements("add", ifname)
	return strings.Join(elms, " ")
}

func (f *SU32Filter) ReplaceLine(ifname string) string {
	elms := f.basicLineElements("replace", ifname)
	return strings.Join(elms, " ")
}

func (f *SU32Filter) DeleteLine(ifname string) string {
	elms := f.basicLineElements("delete", ifname)
	return strings.Join(elms, " ")
}

var (
	ingressMatchReg = regexp.MustCompile(`mirred \(Egress Redirect to device (?P<ifname>[a-z0-9._-]+)\)`)
)

func parseU32Filter(chunks []string) (*SU32Filter, error) {
	m := ingressMatchReg.FindStringSubmatch(strings.Join(chunks, " "))
	if m != nil {
		f := &SU32Filter{}
		f.RedirectDev = m[1]
		return f, nil
	}
	return nil, errors.Wrapf(errors.ErrInvalidFormat, "unknown u32 filter")
}

func parseFilter(chunks []string, parents []IQdisc) (IFilter, error) {
	f, err := parseBaseFilter(chunks, parents)
	if err != nil {
		return nil, errors.Wrapf(err, "parse base filter")
	}
	switch f.Kind {
	case "fw":
		fwFilter, err := parseFwFilter(chunks)
		if err != nil {
			return nil, errors.Wrapf(err, "parse fw filter")
		}
		fwFilter.SBaseTcFilter = f
		return fwFilter, nil
	case "u32":
		u32Filter, err := parseU32Filter(chunks)
		if err != nil {
			return nil, errors.Wrapf(err, "parse u32 filter")
		}
		u32Filter.SBaseTcFilter = f
		return u32Filter, nil
	}
	return nil, errors.Wrapf(errors.ErrInvalidFormat, "unknown filter kind %s", f.Kind)
}

func parseFilterLines(lines []string, parents []IQdisc) ([]IFilter, error) {
	filters := []IFilter{}
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		i++
		for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "filter parent ") {
			line += " " + strings.TrimSpace(lines[i])
			i++
		}
		chunks := strings.Split(line, " ")
		filter, err := parseFilter(chunks, parents)
		if err != nil {
			log.Debugf("parse filter %s: %v", line, err)
		} else {
			filters = append(filters, filter)
		}
	}
	return filters, nil
}
