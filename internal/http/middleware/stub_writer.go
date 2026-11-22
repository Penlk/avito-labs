package middleware

import (
	"net/http"

	"github.com/Penlk/avito-labs/internal/http/domainerror"
)

type bufferWriter struct {
	header  http.Header
	body    []byte
	status  int
	isClose bool
	err     error
}

func NewBufferWriter() *bufferWriter {
	return &bufferWriter{
		header: make(http.Header),
		body:   make([]byte, 0),
		status: http.StatusOK,
	}
}

func (b *bufferWriter) Header() http.Header {
	if b.isClose {
		return b.header.Clone()
	}
	return b.header
}

func (b *bufferWriter) Write(bytes []byte) (int, error) {
	if !b.isClose {
		b.WriteHeader(http.StatusOK)
	}
	b.body = append(b.body, bytes...)
	return len(bytes), nil
}

func (b *bufferWriter) WriteHeader(statusCode int) {
	if !b.isClose {
		b.isClose = true
		b.status = statusCode
		b.header = b.header.Clone()
	}
}

func (b *bufferWriter) RecordError(err error) {
	b.err = err
}

func (b *bufferWriter) WriteToOriginal(w http.ResponseWriter) error {
	domainerror.Record(w, b.err)
	for key, value := range b.Header() {
		w.Header()[key] = append([]string(nil), value...)
	}
	w.WriteHeader(b.status)
	_, err := w.Write(b.body)
	return err
}

var _ http.ResponseWriter = &bufferWriter{}
