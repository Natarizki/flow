package cli

import (
	"bytes"
	"io"
)

func jsonReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}
