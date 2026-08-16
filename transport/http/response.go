package http

type Response interface {
	SetCode(int)
	SetMessage(string)
	SetData(any)
}

type response struct {
	Code    int    `json:"code" xml:"code" yaml:"code"`
	Message string `json:"message" xml:"message" yaml:"message"`
	Data    any    `json:"data,omitempty" xml:"data,omitempty" yaml:"data,omitempty"`
}

func (r *response) SetCode(code int) {
	r.Code = code
}

func (r *response) SetMessage(message string) {
	r.Message = message
}

func (r *response) SetData(data any) {
	r.Data = data
}

func newResponse(code int, message string, data any) Response {
	return &response{
		Code:    code,
		Message: message,
		Data:    data,
	}
}
