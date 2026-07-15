package compression

import (
	"regexp"
)

var (
	htmlCommentRe = regexp.MustCompile(`<!--[\s\S]*?-->`)
	extraSpaceRe  = regexp.MustCompile(`[ \t]{2,}`)
	blankLineRe   = regexp.MustCompile(`\n{2,}`)
	unusedAttrRe  = regexp.MustCompile(`\s(data-[\w-]+)="[^"]*"`)
)

// Level 2: gzip + minify HTML, target ~4x lebih kecil
func compressLevel2(data []byte) ([]byte, []string, map[string]int, error) {
	minified, removedAttrs := minifyHTML(data)

	compressed, methods, err := compressLevel1(minified)
	if err != nil {
		return nil, nil, nil, err
	}

	methods = append(methods, "minify_html")
	removed := map[string]int{
		"html_attributes": removedAttrs,
	}

	return compressed, methods, removed, nil
}

func minifyHTML(data []byte) ([]byte, int) {
	content := string(data)

	content = htmlCommentRe.ReplaceAllString(content, "")

	matches := unusedAttrRe.FindAllString(content, -1)
	content = unusedAttrRe.ReplaceAllString(content, "")

	content = extraSpaceRe.ReplaceAllString(content, " ")
	content = blankLineRe.ReplaceAllString(content, "\n")

	return []byte(content), len(matches)
}
