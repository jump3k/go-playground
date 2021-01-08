package errno

/*
 * errno规则：服务级(1位数) 模块级(2 位数) 具体错误码(4位数)
 */

var (
	// OK
	ErrOK = NewError(0, "OK")

	// 服务级错误
	ErrServer    = NewError(1000001, "服务异常")
	ErrParam     = NewError(1000002, "参数有误")
	ErrSignParam = NewError(1000003, "签名参数有误")

	// 模块级错误码 - 用户模块(01)
	ErrUserPhone   = NewError(2010001, "用户手机号不合法")
	ErrUserCaptcha = NewError(2010002, "用户验证码有误")

	//...
)
