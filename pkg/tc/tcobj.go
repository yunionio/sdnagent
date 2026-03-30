package tc

type IComparable interface {
	CompareBase(IComparable) int
	Compare(IComparable) int
	Equals(IComparable) bool
}

type ITcObj interface {
	Id() string

	IComparable
}

type ITcObjAlter interface {
	DeleteLine(ifname string) string
	AddLine(ifname string) string
	ReplaceLine(ifname string) string
}
