package errno

import "encoding/json"

var _ Error = (*err)(nil)

type Error interface {
	// 设置成功时返回的数据
	WithData(data interface{}) Error

	// 设置当前请求的唯一ID
	WithID(id string) Error

	// 返回JSON格式的错误详情
	String() string

	i() //为了避免被其他包实现
}

type err struct {
	ErrNo  int         `json:"errno"`          //错误码
	ErrMsg string      `json:"errmsg"`         //错误描述
	Data   interface{} `json:"data,omitempty"` //成功时返回的数据
	ID     string      `json:"id,omitempty"`   //当前请求的唯一ID,便于问题定位
}

func NewError(errno int, errmsg string) Error {
	return &err{
		ErrNo:  errno,
		ErrMsg: errmsg,
		Data:   nil,
	}
}

func (e *err) WithData(data interface{}) Error {
	e.Data = data
	return e
}

func (e *err) WithID(id string) Error {
	e.ID = id
	return e
}

func (e *err) String() string {
	raw, _ := json.Marshal(e)
	return string(raw)
}

func (e *err) i() {}
