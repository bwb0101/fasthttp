/*
 * 项目名称：fasthttp
 * 文件名：common.go
 * 日期：2024/03/14 11:52
 * 作者：Ben
 */

package multipart

type MyValidHeader struct {
	ValidFormFileFormat ValidFormFileFormat
	ValidHeadSize       int
}

var MyValidHeaderDefault = MyValidHeader{}