package compression

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var (
	styleBlockRe   = regexp.MustCompile(`(?s)<style[^>]*>(.*?)</style>`)
	cssRuleSplitRe = regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`)
	classAttrRe    = regexp.MustCompile(`\.([a-zA-Z0-9_-]+)`)
	idAttrRe       = regexp.MustCompile(`#([a-zA-Z0-9_-]+)`)
	tagPrefixRe    = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*`)
)

// Level 3: minify + real CSS tree-shaking (cek beneran class/id/tag mana
// yang dipakai di body sebelum hapus rule), target ~6x lebih kecil
func compressLevel3(data []byte) ([]byte, []string, map[string]int, error) {
	minified, removedAttrs := minifyHTML(data)

	usedClasses, usedIDs, usedTags := collectUsedSelectors(minified)
	shaken, removedRules := treeShakeCSS(minified, usedClasses, usedIDs, usedTags)

	compressed, methods, err := compressLevel1(shaken)
	if err != nil {
		return nil, nil, nil, err
	}

	methods = append(methods, "minify_html", "tree_shaking")
	removed := map[string]int{
		"html_attributes":  removedAttrs,
		"unused_css_rules": removedRules,
	}
	return compressed, methods, removed, nil
}

// collectUsedSelectors parse HTML beneran (bukan regex) dan catat semua
// class, id, dan nama tag yang benar-benar dipakai di body (skip isi
// <style>/<script> karena itu bukan "usage", itu definisi).
func collectUsedSelectors(data []byte) (classes, ids, tags map[string]bool) {
	classes = make(map[string]bool)
	ids = make(map[string]bool)
	tags = make(map[string]bool)

	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return classes, ids, tags
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "style" || n.Data == "script" {
				return
			}
			tags[n.Data] = true
			for _, attr := range n.Attr {
				switch attr.Key {
				case "class":
					for _, c := range strings.Fields(attr.Val) {
						classes[c] = true
					}
				case "id":
					ids[attr.Val] = true
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return classes, ids, tags
}

// treeShakeCSS beneran hapus rule yang selector-nya nggak match apapun
// yang ada di collectUsedSelectors.
func treeShakeCSS(data []byte, classes, ids, tags map[string]bool) ([]byte, int) {
	content := string(data)
	removedCount := 0

	content = styleBlockRe.ReplaceAllStringFunc(content, func(block string) string {
		inner := styleBlockRe.FindStringSubmatch(block)[1]
		kept := cssRuleSplitRe.ReplaceAllStringFunc(inner, func(rule string) string {
			parts := cssRuleSplitRe.FindStringSubmatch(rule)
			selectorGroup, decls := parts[1], parts[2]

			anyUsed := false
			for _, sel := range strings.Split(selectorGroup, ",") {
				if selectorIsUsed(strings.TrimSpace(sel), classes, ids, tags) {
					anyUsed = true
					break
				}
			}
			if anyUsed {
				return selectorGroup + "{" + decls + "}"
			}
			removedCount++
			return ""
		})
		return strings.Replace(block, inner, kept, 1)
	})

	return []byte(content), removedCount
}

// selectorIsUsed cocokin 1 simple/compound selector terhadap usage nyata.
// Combinator (spasi, >, +, ~) diambil segmen terakhirnya aja — cukup akurat
// buat mayoritas kasus tanpa perlu full CSS selector engine.
func selectorIsUsed(sel string, classes, ids, tags map[string]bool) bool {
	if sel == "" || sel == "*" {
		return true
	}
	if strings.ContainsAny(sel, "[:@") {
		return true // pseudo-class/attribute selector, jangan hapus, terlalu riskan
	}

	fields := strings.Fields(sel)
	last := fields[len(fields)-1]

	for _, m := range classAttrRe.FindAllStringSubmatch(last, -1) {
		if classes[m[1]] {
			return true
		}
	}
	for _, m := range idAttrRe.FindAllStringSubmatch(last, -1) {
		if ids[m[1]] {
			return true
		}
	}
	if tagName := tagPrefixRe.FindString(last); tagName != "" && tags[tagName] {
		return true
	}
	return false
}
