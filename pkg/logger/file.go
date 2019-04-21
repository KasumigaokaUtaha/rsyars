package logger

import (
	"io"
	"os"

	"github.com/pkg/errors"
)

type fileConsoleWriter struct {
	fd *os.File
}

func (c *fileConsoleWriter) Write(p []byte) (n int, err error) {
	n0, err := c.fd.Write(p)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	err = c.fd.Sync()
	if err != nil {
		return 0, errors.WithStack(err)
	}

	n1, err := os.Stdout.Write(p)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	err = os.Stdout.Sync()
	if err != nil {
		return 0, errors.WithStack(err)
	}

	if n0 != n1 {
		return 0, errors.Errorf("file and stdout write length not equal %d and %d", n0, n1)
	}

	return n, nil
}

func (c *fileConsoleWriter) Close() error {
	return c.fd.Close()
}

func NewWriter(name string) (io.WriteCloser, error) {
	fd, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE, 0)
	if err != nil {
		return nil, errors.WithMessagef(err, "打开日志文件 %s 失败", name)
	}
	return &fileConsoleWriter{fd: fd}, nil
}
