/*
 * 项目名称：fasthttp
 * 文件名：my.go
 * 日期：2023/12/21 18:25
 * 作者：Ben
 */

package multipart

import "errors"

type BodyHeaderCheck func(body []byte, form *Form) (fail bool) // @Ben

var ErrBodyHeaderCheckFail = errors.New("message header does not meet requirements")