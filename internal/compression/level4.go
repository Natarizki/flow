package compression

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"regexp"
)

var dataURIRe = regexp.MustCompile(`data:image/(png|jpe?g|gif);base64,([A-Za-z0-9+/=]+)`)

const reencodeJPEGQuality = 60

// Level 4: minify + tree-shaking + re-encode gambar beneran (decode bytes
// asli, encode ulang ke JPEG kualitas rendah), target ~8x lebih kecil.
// Catatan: cuma gambar yang embedded sebagai base64 data URI yang bisa
// di-reencode di sini karena kita punya raw bytes-nya. Gambar yang
// referensinya external URL (<img src="https://...">) nggak bisa
// direencode tanpa fetch dulu — itu tanggung jawab komponen fetch/cache.
func compressLevel4(data []byte) ([]byte, []string, map[string]int, error) {
	minified, removedAttrs := minifyHTML(data)
	usedClasses, usedIDs, usedTags := collectUsedSelectors(minified)
	shaken, removedRules := treeShakeCSS(minified, usedClasses, usedIDs, usedTags)
	reencoded, reencodedAssets := reencodeDataURIImages(shaken)

	compressed, methods, err := compressLevel1(reencoded)
	if err != nil {
		return nil, nil, nil, err
	}

	methods = append(methods, "minify_html", "tree_shaking", "reencode_assets")
	removed := map[string]int{
		"html_attributes":  removedAttrs,
		"unused_css_rules": removedRules,
		"reencoded_assets": reencodedAssets,
	}
	return compressed, methods, removed, nil
}

// reencodeDataURIImages nemuin data URI gambar, decode beneran, encode
// ulang ke JPEG kualitas rendah, dan cuma dipakai kalau hasilnya beneran
// lebih kecil dari aslinya.
func reencodeDataURIImages(data []byte) ([]byte, int) {
	content := string(data)
	count := 0

	content = dataURIRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := dataURIRe.FindStringSubmatch(match)
		raw, err := base64.StdEncoding.DecodeString(sub[2])
		if err != nil {
			return match
		}

		img, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			return match
		}

		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: reencodeJPEGQuality}); err != nil {
			return match
		}
		if buf.Len() >= len(raw) {
			return match
		}

		count++
		return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	})

	return []byte(content), count
}
