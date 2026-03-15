package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vmihailenco/msgpack/v5"
)

// MsgPack — middleware который перехватывает ответ и сериализует его
// в MessagePack если клиент прислал Accept: application/msgpack
//
// Использование в main.go:
//   private.Use(middleware.MsgPack())
//
// Клиент должен слать заголовок:
//   Accept: application/msgpack

const MsgPackMIME = "application/msgpack"

func MsgPack() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Accept") != MsgPackMIME {
			c.Next()
			return
		}

		blw := &msgpackWriter{
			body:           bytes.NewBuffer(nil),
			ResponseWriter: c.Writer,
		}
		c.Writer = blw

		c.Next()

		body := blw.body.Bytes()
		if len(body) == 0 {
			if blw.status != 0 {
				blw.ResponseWriter.WriteHeader(blw.status)
			}
			return
		}

		// JSON → interface{}
		var v interface{}
		if err := json.Unmarshal(body, &v); err != nil {
			// не JSON — отдаём как есть
			if blw.status != 0 {
				blw.ResponseWriter.WriteHeader(blw.status)
			}
			blw.ResponseWriter.Write(body)
			return
		}

		// interface{} → msgpack
		packed, err := msgpack.Marshal(v)
		if err != nil {
			if blw.status != 0 {
				blw.ResponseWriter.WriteHeader(blw.status)
			}
			blw.ResponseWriter.Write(body)
			return
		}

		status := blw.status
		if status == 0 {
			status = http.StatusOK
		}

		blw.ResponseWriter.Header().Set("Content-Type", MsgPackMIME)
		blw.ResponseWriter.WriteHeader(status)
		blw.ResponseWriter.Write(packed)
	}
}

// msgpackWriter — буферизует ответ хендлера
type msgpackWriter struct {
	gin.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (w *msgpackWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *msgpackWriter) WriteHeader(status int) {
	w.status = status
}

func (w *msgpackWriter) WriteString(s string) (int, error) {
	return w.body.WriteString(s)
}