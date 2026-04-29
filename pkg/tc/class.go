package tc

import (
	"fmt"
	"strconv"
	"strings"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
)

const (
	ErrParentClassNotFound = errors.Error("ParentClassNotFound")
)

/*
 * // add
 * tc class add dev br0 parent 1: classid 1:1 htb rate 10000mbit ceil 10000mbit
 * tc class add dev br0 parent 1:1 classid 1:2 htb rate 1000mbit ceil 10000mbit
 * tc class add dev br0 parent 1:1 classid 1:3 htb rate 100mbit ceil 100mbit
 * // show
 * class htb 1:1 root rate 10Gbit ceil 10Gbit burst 0b cburst 0b
 * class htb 1:2 parent 1:1 prio 0 rate 1Gbit ceil 10Gbit burst 1375b cburst 0b
 * class htb 1:3 parent 1:1 prio 0 rate 100Mbit ceil 100Mbit burst 1600b cburst 1600b
 */
type IClass interface {
	ITcObj
	ITcObjAlter

	Base() *SBaseTcClass

	IsRoot() bool
}

type SBaseTcClass struct {
	Kind    string
	ClassId string
	Parent  IClass
}

func (q *SBaseTcClass) Id() string {
	return q.ClassId
}

func (q *SBaseTcClass) IsRoot() bool {
	return q.Parent == nil
}

func convertClassId(classId string) (uint64, uint64) {
	var major, minor uint64
	parts := strings.Split(classId, ":")
	if len(parts) > 0 {
		major, _ = strconv.ParseUint(parts[0], 16, 64)
	}
	if len(parts) > 1 {
		minor, _ = strconv.ParseUint(parts[1], 16, 64)
	}
	return major, minor
}

func compareClassId(classId1 string, classId2 string) int {
	major1, minor1 := convertClassId(classId1)
	major2, minor2 := convertClassId(classId2)
	if major1 < major2 {
		return -1
	} else if major1 > major2 {
		return 1
	}
	if minor1 < minor2 {
		return -1
	} else if minor1 > minor2 {
		return 1
	}
	return 0
}

func (q *SBaseTcClass) Compare(qi IComparable) int {
	q2, ok := qi.(*SBaseTcClass)
	if !ok {
		return -1
	}
	if q.ClassId != q2.ClassId {
		return compareClassId(q.ClassId, q2.ClassId)
	}
	if q.Kind != q2.Kind {
		return strings.Compare(q.Kind, q2.Kind)
	}
	if q.Parent == nil && q2.Parent != nil {
		return -1
	}
	if q.Parent != nil && q2.Parent == nil {
		return 1
	}
	if q.Parent != nil && q2.Parent != nil && q.Parent.Id() != q2.Parent.Id() {
		return compareClassId(q.Parent.Id(), q2.Parent.Id())
	}
	return 0
}

func (q *SBaseTcClass) CompareBase(qi IComparable) int {
	return q.Compare(qi)
}

func (q *SBaseTcClass) Equals(qi IComparable) bool {
	return q.Compare(qi) == 0
}

func (cls *SBaseTcClass) AddLine(ifname string) string {
	elms := cls.basicLineElements("add", ifname)
	return strings.Join(elms, " ")
}

func (cls *SBaseTcClass) ReplaceLine(ifname string) string {
	elms := cls.basicLineElements("replace", ifname)
	return strings.Join(elms, " ")
}

func (cls *SBaseTcClass) DeleteLine(ifname string) string {
	elms := cls.basicLineElements("delete", ifname)
	return strings.Join(elms, " ")
}

func (cls *SBaseTcClass) Base() *SBaseTcClass {
	return cls
}

// tc filter replace dev eth0 parent 1: prio 1 handle 10: flower src_ip 192.168.1.10 action mirred egress redirect dev ifb0
func (f *SBaseTcClass) basicLineElements(action string, ifname string) []string {
	elms := []string{"class", action, "dev", ifname}
	if f.Parent != nil {
		elms = append(elms, "parent", f.Parent.Id())
	} else {
		elms = append(elms, "parent", "1:")
	}
	elms = append(elms, "classid", f.ClassId, f.Kind)
	return elms
}

func parseBaseClass(chunks []string, existingClasses []IClass) (*SBaseTcClass, error) {
	cls := &SBaseTcClass{}
	for i := 0; i < len(chunks); {
		c := chunks[i]
		switch c {
		case "class":
			if i < len(chunks)-2 {
				cls.Kind = chunks[i+1]
				cls.ClassId = chunks[i+2]
				i += 3
			} else {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "eol before getting class id")
			}
		case "parent":
			if i+1 >= len(chunks) {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "eol before getting parent")
			}
			parentId := chunks[i+1]
			for _, pcls := range existingClasses {
				if pcls.Id() == parentId {
					cls.Parent = pcls
					break
				}
			}
			if cls.Parent == nil {
				return cls, errors.Wrapf(ErrParentClassNotFound, "parent class %s not found", parentId)
			}
			i += 2
		default:
			i++
		}
	}
	return cls, nil
}

// rate 10Gbit ceil 10Gbit burst 0b cburst 0b
type SHtbClass struct {
	*SBaseTcClass
	Rate uint64
	Ceil uint64
}

func (q *SHtbClass) Base() *SBaseTcClass {
	return q.SBaseTcClass
}

func (q *SHtbClass) Compare(itc IComparable) int {
	baseCls, ok := itc.(IClass)
	if !ok {
		return -1
	}
	baseCmp := q.Base().Compare(baseCls.Base())
	if baseCmp != 0 {
		return baseCmp
	}
	q2 := baseCls.(*SHtbClass)
	if q.Rate < q2.Rate {
		return -1
	} else if q.Rate > q2.Rate {
		return 1
	}
	if q.Ceil < q2.Ceil {
		return -1
	} else if q.Ceil > q2.Ceil {
		return 1
	}
	return 0
}

func (q *SHtbClass) CompareBase(qi IComparable) int {
	return q.Base().CompareBase(qi.(IClass).Base())
}

func (q *SHtbClass) Equals(qi IComparable) bool {
	return q.Compare(qi) == 0
}

func (cls *SHtbClass) basicLineElements(action string, ifname string) []string {
	elms := cls.SBaseTcClass.basicLineElements(action, ifname)
	elms = append(elms, "rate", PrintRate(cls.Rate))
	elms = append(elms, "ceil", PrintRate(cls.Ceil))
	return elms
}

func (cls *SHtbClass) AddLine(ifname string) string {
	elms := cls.basicLineElements("add", ifname)
	return strings.Join(elms, " ")
}

func (cls *SHtbClass) ReplaceLine(ifname string) string {
	elms := cls.basicLineElements("replace", ifname)
	return strings.Join(elms, " ")
}

func (cls *SHtbClass) DeleteLine(ifname string) string {
	elms := cls.basicLineElements("delete", ifname)
	return strings.Join(elms, " ")
}

func parseHtbClass(chunks []string) (*SHtbClass, error) {
	cls := &SHtbClass{}
	for i := 0; i < len(chunks); {
		c := chunks[i]
		switch c {
		case "rate":
			if i+1 >= len(chunks) {
				return nil, fmt.Errorf("eol getting rate")
			}
			bytesPerSec, err := ParseRate(chunks[i+1])
			if err != nil {
				return nil, err
			}
			cls.Rate = bytesPerSec
			i += 2
		case "ceil":
			if i+1 >= len(chunks) {
				return nil, fmt.Errorf("eol getting ceil")
			}
			bytesPerSec, err := ParseRate(chunks[i+1])
			if err != nil {
				return nil, err
			}
			cls.Ceil = bytesPerSec
			i += 2
		default:
			i++
		}
	}
	return cls, nil
}

func parseClass(chunks []string, existingClasses []IClass) (IClass, error) {
	cls, err := parseBaseClass(chunks, existingClasses)
	if err != nil {
		return cls, errors.Wrapf(err, "parse base class")
	}
	if cls.Kind == "htb" {
		htbCls, err := parseHtbClass(chunks)
		if err != nil {
			return nil, errors.Wrapf(err, "parse htb class")
		}
		htbCls.SBaseTcClass = cls
		return htbCls, nil
	}
	return nil, fmt.Errorf("unknown class kind %s", cls.Kind)
}

func parseClassLines(lines []string) ([]IClass, error) {
	classes := []IClass{}
	leftoverLines := lines
	visitedClasses := make(map[string]struct{})
	for len(leftoverLines) > 0 {
		var noParentsLines []string
		for _, line := range leftoverLines {
			chunks := strings.Split(strings.TrimSpace(line), " ")
			cls, err := parseClass(chunks, classes)
			if err != nil {
				if errors.Cause(err) == ErrParentClassNotFound {
					if _, ok := visitedClasses[cls.Id()]; !ok {
						noParentsLines = append(noParentsLines, line)
						visitedClasses[cls.Id()] = struct{}{}
					}
				} else {
					log.Debugf("parse class %s: %v", line, err)
				}
			} else {
				classes = append(classes, cls)
			}
		}
		leftoverLines = noParentsLines
	}
	return classes, nil
}
