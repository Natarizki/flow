package compression

import (
	"bytes"

	"github.com/klauspost/compress/zstd"
)

// Level 0: lossless, tetap dikemas pakai zstd tapi tanpa modifikasi konten
func compressLevel0(data []byte) ([]byte, []string, error) {
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return nil, nil, err
	}
	defer encoder.Close()

	compressed := encoder.EncodeAll(data, nil)
	return compressed, []string{"zstd_lossless"}, nil
}

// Level 1: gzip/zstd standar, target ~2x lebih kecil
func compressLevel1(data []byte) ([]byte, []string, error) {
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, nil, err
	}
	defer encoder.Close()

	compressed := encoder.EncodeAll(data, nil)
	return compressed, []string{"gzip"}, nil
}

func decompressZstd(data []byte) ([]byte, error) {
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()

	return decoder.DecodeAll(data, nil)
}

func DecodeBuffer(data []byte) (*bytes.Buffer, error) {
	out, err := decompressZstd(data)
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(out), nil
}
