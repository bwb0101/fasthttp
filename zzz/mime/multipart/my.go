/*
 * 项目名称：fasthttp
 * 文件名：my.go
 * 日期：2023/12/21 18:25
 * 作者：Ben
 */

package multipart

type BodyHeaderCheck func(body []byte, fileName string, form *Form) (err error) // @Ben