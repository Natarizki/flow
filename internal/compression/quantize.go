package compression

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Natarizki/flow/pkg/utils"
)

const (
	MagicNumber   = "FLOW"
	FormatVersion = 1
	FixedQuality  = 89
)

type QuantLevel int

const (
	LevelLossless QuantLevel = 0
	Level1        QuantLevel = 1 // 2x smaller
	Level2        QuantLevel = 2 // 4x smaller
	Level3        QuantLevel = 3 // 6x smaller
	Level4        QuantLevel = 4 // 8x smaller
)

type OriginalMeta struct {
	URL         string `json:"url,omitempty"`
	Size        int64  `json:"size"`
	Checksum    string `json:"checksum"`
	ContentType string `json:"content_type,omitempty"`
}

type CompressionMeta struct {
	Level   int            `json:"level"`
	Quality int            `json:"quality"`
	Ratio   float64        `json:"ratio"`
	Methods []string       `json:"methods"`
	Removed map[string]int `json:"removed,omitempty"`
}

type FlowMetadata struct {
	Original    OriginalMeta    `json:"original"`
	Compression CompressionMeta `json:"compression"`
}

type FlowFile struct {
	Version            byte
	Level              QuantLevel
	Quality            byte
	Flags              uint16
	OriginalSize       uint32
	CompressedSize     uint32
	OriginalChecksum   [8]byte
	CompressedChecksum [8]byte
	Metadata           FlowMetadata
	Data               []byte
}

func Encode(f *FlowFile) ([]byte, error) {
	metaJSON, err := json.Marshal(f.Metadata)
	if err != nil {
		return nil, utils.WrapError("ENCODE", "failed to marshal metadata", err)
	}

	buf := new(bytes.Buffer)
	buf.WriteString(MagicNumber)
	buf.WriteByte(FormatVersion)
	buf.WriteByte(byte(f.Level))
	buf.WriteByte(FixedQuality)
	binary.Write(buf, binary.BigEndian, f.Flags)
	binary.Write(buf, binary.BigEndian, f.OriginalSize)
	binary.Write(buf, binary.BigEndian, f.CompressedSize)
	buf.Write(f.OriginalChecksum[:])
	buf.Write(f.CompressedChecksum[:])
	binary.Write(buf, binary.BigEndian, uint32(len(metaJSON)))
	buf.Write(metaJSON)
	buf.Write(f.Data)

	return buf.Bytes(), nil
}

func Decode(raw []byte) (*FlowFile, error) {
	if len(raw) < 4 || string(raw[:4]) != MagicNumber {
		return nil, utils.ErrInvalidFormat
	}

	r := bytes.NewReader(raw[4:])
	f := &FlowFile{}

	binary.Read(r, binary.BigEndian, &f.Version)
	var level byte
	binary.Read(r, binary.BigEndian, &level)
	f.Level = QuantLevel(level)
	binary.Read(r, binary.BigEndian, &f.Quality)
	binary.Read(r, binary.BigEndian, &f.Flags)
	binary.Read(r, binary.BigEndian, &f.OriginalSize)
	binary.Read(r, binary.BigEndian, &f.CompressedSize)
	r.Read(f.OriginalChecksum[:])
	r.Read(f.CompressedChecksum[:])

	var metaLen uint32
	binary.Read(r, binary.BigEndian, &metaLen)

	metaJSON := make([]byte, metaLen)
	if _, err := r.Read(metaJSON); err != nil {
		return nil, utils.WrapError("DECODE", "failed to read metadata", err)
	}
	if err := json.Unmarshal(metaJSON, &f.Metadata); err != nil {
		return nil, utils.WrapError("DECODE", "failed to parse metadata", err)
	}

	remaining := make([]byte, r.Len())
	r.Read(remaining)
	f.Data = remaining

	return f, nil
}

func checksumPartial(hash string) [8]byte {
	var out [8]byte
	copy(out[:], hash[:8])
	return out
}

func ChecksumPartial(hash string) [8]byte {
	return checksumPartial(hash)
}

func ExtensionFor(level QuantLevel) string {
	if level == LevelLossless {
		return ".flow"
	}
	return fmt.Sprintf(".flow.%d", level)
}

// Compress adalah dispatcher utama. Kalau content-type-nya gambar, lewat
// jalur compressImage (decode+reencode beneran). Selain itu (HTML/text)
// lewat jalur minify+tree-shake per level.
func Compress(data []byte, level QuantLevel, url, contentType string) (*FlowFile, error) {
	var compressed []byte
	var methods []string
	var removed map[string]int
	var err error

	isImage := strings.HasPrefix(contentType, "image/")

	switch {
	case isImage:
		compressed, methods, err = compressImage(data, level)
	case level == LevelLossless:
		compressed, methods, err = compressLevel0(data)
	case level == Level1:
		compressed, methods, err = compressLevel1(data)
	case level == Level2:
		compressed, methods, removed, err = compressLevel2(data)
	case level == Level3:
		compressed, methods, removed, err = compressLevel3(data)
	case level == Level4:
		compressed, methods, removed, err = compressLevel4(data)
	default:
		return nil, utils.NewError("INVALID_LEVEL", "quantization level must be 0-4")
	}
	if err != nil {
		return nil, err
	}

	origHash := utils.HashBytes(data)
	compHash := utils.HashBytes(compressed)
	ratio := float64(len(compressed)) / float64(len(data))

	return &FlowFile{
		Version:            FormatVersion,
		Level:              level,
		Quality:            FixedQuality,
		OriginalSize:       uint32(len(data)),
		CompressedSize:     uint32(len(compressed)),
		OriginalChecksum:   checksumPartial(origHash),
		CompressedChecksum: checksumPartial(compHash),
		Metadata: FlowMetadata{
			Original: OriginalMeta{
				URL:         url,
				Size:        int64(len(data)),
				Checksum:    origHash,
				ContentType: contentType,
			},
			Compression: CompressionMeta{
				Level:   int(level),
				Quality: FixedQuality,
				Ratio:   ratio,
				Methods: methods,
				Removed: removed,
			},
		},
		Data: compressed,
	}, nil
}
