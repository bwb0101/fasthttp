/*
 * 项目名称：fasthttp
 * 文件名：http_my.go
 * 日期：2024/02/27 19:00
 * 作者：Ben
 */

package fasthttp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp/zzz/mime/multipart"
)

var strCRLFLen = len(strCRLF)

// 如果是form，返回formSize指定的大小
//
// body不应该把form的数据都读取到内存中，比如一个很大的文件
func (req *Request) BodyLimit(limitFormSize int) []byte {
	if req.bodyRaw != nil {
		return req.bodyRaw
	} else if req.onlyMultipartForm() {
		body, err := req.marshalMultipartForm(limitFormSize)
		if err != nil {
			return []byte(err.Error())
		}
		return body
	}
	return req.bodyBytes()
}

func (req *Request) marshalMultipartForm(limitFormSize int) ([]byte, error) {
	var buf bytebufferpool.ByteBuffer
	if err := writeMultipartForm(&buf, req.multipartForm, req.multipartFormBoundary, limitFormSize); err != nil {
		return nil, err
	}
	return buf.B, nil
}

func writeMultipartForm(w *bytebufferpool.ByteBuffer, f *multipart.Form, boundary string, limitFormSize int) error {
	// Do not care about memory allocations here, since multipart
	// form processing is slow.
	if len(boundary) == 0 {
		return errors.New("form boundary cannot be empty")
	}
	//
	mw := multipart.NewWriter(w)
	if err := mw.SetBoundary(boundary); err != nil {
		return fmt.Errorf("cannot use form boundary %q: %w", boundary, err)
	}

	// marshal values
	for k, vv := range f.Value {
		for _, v := range vv {
			if err := mw.WriteField(k, v); err != nil {
				return fmt.Errorf("cannot write form field %q value %q: %w", k, v, err)
			}
		}
	}

	if limitFormSize < 64 {
		limitFormSize = 1024
	}
	if len(w.B) >= limitFormSize {
		return nil
	}

	// marshal files
	for k, fvv := range f.File {
		for _, fv := range fvv {
			if len(w.B) >= limitFormSize {
				break
			}
			vw, err := mw.CreatePart(fv.Header)
			if err != nil {
				return fmt.Errorf("cannot create form file %q (%q): %w", k, fv.Filename, err)
			}
			fh, err := fv.Open()
			if err != nil {
				return fmt.Errorf("cannot open form file %q (%q): %w", k, fv.Filename, err)
			}
			if _, err = limitCopyZeroAlloc(vw, fh, limitFormSize-len(w.B)); err != nil {
				_ = fh.Close()
				return fmt.Errorf("error when copying form file %q (%q): %w", k, fv.Filename, err)
			}
			if err = fh.Close(); err != nil {
				return fmt.Errorf("cannot close form file %q (%q): %w", k, fv.Filename, err)
			}
		}
	}

	if err := mw.Close(); err != nil {
		return fmt.Errorf("error when closing multipart form writer: %w", err)
	}

	return nil
}

func limitCopyZeroAlloc(w io.Writer, r io.Reader, limit int) (int64, error) {
	vbuf := copyBufPool.Get()
	buf := vbuf.([]byte)
	n, err := io.CopyBuffer(w, &myLimitReader{r: r, limit: limit}, buf)
	copyBufPool.Put(vbuf)
	return n, err
}

func (req *Request) readBodyChunked2(r *bufio.Reader, maxBodySize int, dst []byte) ([]byte, error) {
	if len(dst) > 0 {
		// data integrity might be in danger. No idea what we received,
		// but nothing we should write to.
		panic("BUG: expected zero-length buffer")
	}
	if boundary := b2s(req.Header.MultipartFormBoundary()); boundary == "" {
		if maxBodySize > defaultMaxInMemoryFileSize {
			maxBodySize = defaultMaxInMemoryFileSize // @Ben 不允许以这种形式上传大文件，会把整个文件流都读取到内存中
		}
		for {
			chunkSize, err := parseChunkSize(r)
			if err != nil {
				return dst, err
			}
			if chunkSize == 0 {
				return dst, err
			}
			if maxBodySize > 0 && len(dst)+chunkSize > maxBodySize {
				return dst, ErrBodyTooLarge
			}
			dst, err = appendBodyFixedSize(r, dst, chunkSize+strCRLFLen)
			if err != nil {
				return dst, err
			}
			if !bytes.Equal(dst[len(dst)-strCRLFLen:], strCRLF) {
				return dst, ErrBrokenChunk{
					error: fmt.Errorf("cannot find crlf at the end of chunk"),
				}
			}
			dst = dst[:len(dst)-strCRLFLen]
		}
	} else {
		req.multipartFormBoundary = boundary
		mr := multipart.NewReader(&chunkedFormReader{r: r, limitSize: maxBodySize, buf: make([]byte, 4098)}, boundary)
		var err error
		req.multipartForm, err = mr.ReadForm(req.Header.myValid, int64(defaultMaxInMemoryFileSize))
		return append(dst, 0), err // 返回一个字节结果
	}
}

type chunkedFormReader struct {
	r         *bufio.Reader
	limitSize int
	rs        int
	chunkSize int
	rd        int
	wt        int
	buf       []byte
}

func (r *chunkedFormReader) Read(p []byte) (n int, err error) {
	if r.wt > r.rd {
		copy(p, r.buf[r.rd:r.wt]) // 返回剩余的
		n = r.wt - r.rd
		r.wt = 0
		r.rd = 0
		return
	}
	//
	chunkSize := r.chunkSize
	if chunkSize == 0 {
		chunkSize, err = parseChunkSize(r.r)
		if err != nil {
			return 0, err
		}
		if chunkSize == 0 {
			return 0, err
		}
		if r.limitSize > 0 && r.rs+chunkSize > r.limitSize {
			return 0, ErrBodyTooLarge
		}
		r.chunkSize = chunkSize
	}
	const bs = 4096
	if chunkSize > bs { // 读取的大于4096，需要分多次读取
		r.chunkSize -= bs
		r.rs += bs
		n = bs
		chunkSize = bs
	} else {
		n = chunkSize
		r.chunkSize = 0 // 全部读取完
		r.rs += chunkSize
		chunkSize += strCRLFLen // 读取尾部结束标记
	}
	r.wt = n
	err = r.appendBodyFixedSize(r.buf[:chunkSize], chunkSize)
	if err != nil {
		return 0, err
	}
	if r.chunkSize == 0 && !bytes.Equal(r.buf[n:chunkSize], strCRLF) {
		return 0, ErrBrokenChunk{
			error: fmt.Errorf("cannot find crlf at the end of chunk"),
		}
	} else {
		n = copy(p, r.buf[:n])
		r.rd = n
	}
	return
}

func (r *chunkedFormReader) appendBodyFixedSize(dst []byte, n int) error {
	if n == 0 {
		return nil
	}
	offset := 0
	for {
		nn, err := r.r.Read(dst[offset:])
		if nn <= 0 {
			switch {
			case errors.Is(err, io.EOF):
				return io.ErrUnexpectedEOF
			case err != nil:
				return err
			default:
				return fmt.Errorf("bufio.Read() returned (%d, nil)", nn)
			}
		}
		offset += nn
		if n == offset {
			return nil
		}
	}
}

type myLimitReader struct {
	limit int
	n     int
	r     io.Reader
}

func (r *myLimitReader) Read(p []byte) (n int, err error) {
	if n, err = r.r.Read(p); err == nil {
		r.n += n
		if r.n >= r.limit {
			n = r.limit
			err = io.EOF
		}
	}
	return
}