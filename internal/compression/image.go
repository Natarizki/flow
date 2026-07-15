package compression

import (
	"bytes"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
)

// imageQualityByLevel: kualitas JPEG re-encode per level kuantisasi.
// Ini yang beneran dipakai kalau content-type-nya gambar langsung
// (bukan HTML yang isinya <img>).
var imageQualityByLevel = map[QuantLevel]int{
	Level1: 85,
	Level2: 75,
	Level3: 65,
	Level4: 50,
}

// compressImage decode gambar raster apapun (png/jpg/gif) dan encode ulang
// jadi JPEG di quality sesuai level. Kalau formatnya nggak bisa didecode
// (svg, webp, dll), fallback ke zstd wrap biasa (masih lossless).
func compressImage(data []byte, level QuantLevel) ([]byte, []string, error) {
	if level == LevelLossless {
		return compressLevel0(data)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return compressLevel1(data)
	}

	quality := imageQualityByLevel[level]
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return compressLevel1(data)
	}

	return buf.Bytes(), []string{"image_reencode_jpeg"}, nil
}
