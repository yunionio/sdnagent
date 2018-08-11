package proto

type CommonResponse interface {
	GetCode() uint32
	GetMesg() string
}
