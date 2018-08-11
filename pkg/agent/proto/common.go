package pb

type CommonResponse interface {
	GetCode() uint32
	GetMesg() string
}
