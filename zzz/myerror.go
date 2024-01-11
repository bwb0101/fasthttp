/*
 * 项目名称：fasthttp
 * 文件名：myerror.go
 * 日期：2024/01/11 15:29
 * 作者：Ben
 */

package zzz

import (
	"errors"
)

var merr = errors.New("my error")

type FastHttpMyHeaderCheckError struct {
	Code    int32
	CodeStr string
	error
}

func NewFastHttpMyHeaderCheckError(code int32) *FastHttpMyHeaderCheckError {
	return &FastHttpMyHeaderCheckError{
		Code:  code,
		error: merr,
	}
}

func NewFastHttpMyHeaderCheckErrorStr(str string) *FastHttpMyHeaderCheckError {
	return &FastHttpMyHeaderCheckError{
		CodeStr: str,
		error:   merr,
	}
}