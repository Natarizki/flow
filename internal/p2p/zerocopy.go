package p2p

import (
	"io"
	"os"
)

// SendFileZeroCopy transfers a file's contents to a writer using the
// kernel's sendfile-family syscall path where possible. On Linux (which
// Termux's kernel is), io.Copy from an *os.File automatically uses
// io.Copy's src.(io.WriterTo) / dst.(io.ReaderFrom) fast paths — when
// the destination is a *net.TCPConn, Go's net package implements
// ReadFrom using sendfile(2) under the hood, avoiding a userspace
// buffer copy entirely. This wrapper just makes that path explicit and
// documented, rather than accidentally relying on it.
func SendFileZeroCopy(path string, dst io.Writer) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// io.Copy checks if dst implements io.ReaderFrom (net.TCPConn does)
	// and if so calls dst.ReadFrom(f) directly, which for a *os.File
	// source on Linux triggers the sendfile(2) syscall path in net's
	// internal implementation — zero userspace copies of the file data.
	return io.Copy(dst, f)
}

// ChunkReaderAt lets a Chunk's data be read via io.ReaderAt without an
// intermediate copy, useful when serving partial/ranged chunk reads
// directly from a memory-mapped or already-loaded buffer.
type ChunkReaderAt struct {
	data []byte
}

func NewChunkReaderAt(data []byte) *ChunkReaderAt {
	return &ChunkReaderAt{data: data}
}

func (c *ChunkReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(c.data)) {
		return 0, io.EOF
	}
	n := copy(p, c.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
