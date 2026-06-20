package xsdregex

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NativePattern rewrites the XML Schema regex constructs Cambium supports into
// equivalent Go regexp syntax while preserving full-string matching semantics
// at the call site.
func NativePattern(pattern string) string {
	runes := []rune(pattern)
	var out strings.Builder
	inClass := false
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			if replacement, end, ok := nativeEscapeReplacement(runes, i, inClass); ok {
				out.WriteString(replacement)
				i = end
				continue
			}
			out.WriteRune(runes[i])
			if i+1 < len(runes) {
				i++
				out.WriteRune(runes[i])
			}
		case '[':
			if replacement, end, ok := nativeClassReplacement(runes, i); ok {
				out.WriteString(replacement)
				i = end
				continue
			}
			inClass = true
			out.WriteRune(runes[i])
		case ']':
			inClass = false
			out.WriteRune(runes[i])
		case '.':
			if !inClass {
				out.WriteString(`[\x{0}-\x{9}\x{B}-\x{C}\x{E}-\x{10FFFF}]`)
				continue
			}
			out.WriteRune(runes[i])
		case '^':
			if !inClass {
				out.WriteString(`\x{5E}`)
				continue
			}
			out.WriteRune(runes[i])
		case '$':
			if !inClass {
				out.WriteString(`\x{24}`)
				continue
			}
			out.WriteRune(runes[i])
		default:
			out.WriteRune(runes[i])
		}
	}
	return out.String()
}

// UnsupportedNativeSyntax reports XML Schema regex forms that are valid enough
// to parse but cannot be represented by Cambium's native Go regexp bridge.
func UnsupportedNativeSyntax(pattern string) string {
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			i = skipEscape(runes, i)
		case '[':
			end, unsupported := unsupportedNativeClassSyntax(runes, i)
			if unsupported != "" {
				return unsupported
			}
			i = end
		}
	}
	return ""
}

type runeRange struct {
	lo rune
	hi rune
}

var unicodeBlockRanges = map[string][]runeRange{
	"BasicLatin":                         {{0x0000, 0x007F}},
	"Latin-1Supplement":                  {{0x0080, 0x00FF}},
	"LatinExtended-A":                    {{0x0100, 0x017F}},
	"LatinExtended-B":                    {{0x0180, 0x024F}},
	"IPAExtensions":                      {{0x0250, 0x02AF}},
	"SpacingModifierLetters":             {{0x02B0, 0x02FF}},
	"CombiningDiacriticalMarks":          {{0x0300, 0x036F}},
	"Greek":                              {{0x0370, 0x03FF}},
	"Cyrillic":                           {{0x0400, 0x04FF}},
	"Armenian":                           {{0x0530, 0x058F}},
	"Hebrew":                             {{0x0590, 0x05FF}},
	"Arabic":                             {{0x0600, 0x06FF}},
	"Syriac":                             {{0x0700, 0x074F}},
	"Thaana":                             {{0x0780, 0x07BF}},
	"Devanagari":                         {{0x0900, 0x097F}},
	"Bengali":                            {{0x0980, 0x09FF}},
	"Gurmukhi":                           {{0x0A00, 0x0A7F}},
	"Gujarati":                           {{0x0A80, 0x0AFF}},
	"Oriya":                              {{0x0B00, 0x0B7F}},
	"Tamil":                              {{0x0B80, 0x0BFF}},
	"Telugu":                             {{0x0C00, 0x0C7F}},
	"Kannada":                            {{0x0C80, 0x0CFF}},
	"Malayalam":                          {{0x0D00, 0x0D7F}},
	"Sinhala":                            {{0x0D80, 0x0DFF}},
	"Thai":                               {{0x0E00, 0x0E7F}},
	"Lao":                                {{0x0E80, 0x0EFF}},
	"Tibetan":                            {{0x0F00, 0x0FFF}},
	"Myanmar":                            {{0x1000, 0x109F}},
	"Georgian":                           {{0x10A0, 0x10FF}},
	"HangulJamo":                         {{0x1100, 0x11FF}},
	"Ethiopic":                           {{0x1200, 0x137F}},
	"Cherokee":                           {{0x13A0, 0x13FF}},
	"UnifiedCanadianAboriginalSyllabics": {{0x1400, 0x167F}},
	"Ogham":                              {{0x1680, 0x169F}},
	"Runic":                              {{0x16A0, 0x16FF}},
	"Khmer":                              {{0x1780, 0x17FF}},
	"Mongolian":                          {{0x1800, 0x18AF}},
	"LatinExtendedAdditional":            {{0x1E00, 0x1EFF}},
	"GreekExtended":                      {{0x1F00, 0x1FFF}},
	"GeneralPunctuation":                 {{0x2000, 0x206F}},
	"SuperscriptsandSubscripts":          {{0x2070, 0x209F}},
	"CurrencySymbols":                    {{0x20A0, 0x20CF}},
	"CombiningMarksforSymbols":           {{0x20D0, 0x20FF}},
	"LetterlikeSymbols":                  {{0x2100, 0x214F}},
	"NumberForms":                        {{0x2150, 0x218F}},
	"Arrows":                             {{0x2190, 0x21FF}},
	"MathematicalOperators":              {{0x2200, 0x22FF}},
	"MiscellaneousTechnical":             {{0x2300, 0x23FF}},
	"ControlPictures":                    {{0x2400, 0x243F}},
	"OpticalCharacterRecognition":        {{0x2440, 0x245F}},
	"EnclosedAlphanumerics":              {{0x2460, 0x24FF}},
	"BoxDrawing":                         {{0x2500, 0x257F}},
	"BlockElements":                      {{0x2580, 0x259F}},
	"GeometricShapes":                    {{0x25A0, 0x25FF}},
	"MiscellaneousSymbols":               {{0x2600, 0x26FF}},
	"Dingbats":                           {{0x2700, 0x27BF}},
	"BraillePatterns":                    {{0x2800, 0x28FF}},
	"CJKRadicalsSupplement":              {{0x2E80, 0x2EFF}},
	"KangxiRadicals":                     {{0x2F00, 0x2FDF}},
	"IdeographicDescriptionCharacters":   {{0x2FF0, 0x2FFF}},
	"CJKSymbolsandPunctuation":           {{0x3000, 0x303F}},
	"Hiragana":                           {{0x3040, 0x309F}},
	"Katakana":                           {{0x30A0, 0x30FF}},
	"Bopomofo":                           {{0x3100, 0x312F}},
	"HangulCompatibilityJamo":            {{0x3130, 0x318F}},
	"Kanbun":                             {{0x3190, 0x319F}},
	"BopomofoExtended":                   {{0x31A0, 0x31BF}},
	"EnclosedCJKLettersandMonths":        {{0x3200, 0x32FF}},
	"CJKCompatibility":                   {{0x3300, 0x33FF}},
	"CJKUnifiedIdeographsExtensionA":     {{0x3400, 0x4DB5}},
	"CJKUnifiedIdeographs":               {{0x4E00, 0x9FFF}},
	"YiSyllables":                        {{0xA000, 0xA48F}},
	"YiRadicals":                         {{0xA490, 0xA4CF}},
	"HangulSyllables":                    {{0xAC00, 0xD7A3}},
	"PrivateUse":                         {{0xE000, 0xF8FF}},
	"CJKCompatibilityIdeographs":         {{0xF900, 0xFAFF}},
	"AlphabeticPresentationForms":        {{0xFB00, 0xFB4F}},
	"ArabicPresentationForms-A":          {{0xFB50, 0xFDFF}},
	"CombiningHalfMarks":                 {{0xFE20, 0xFE2F}},
	"CJKCompatibilityForms":              {{0xFE30, 0xFE4F}},
	"SmallFormVariants":                  {{0xFE50, 0xFE6F}},
	"ArabicPresentationForms-B":          {{0xFE70, 0xFEFE}},
	"HalfwidthandFullwidthForms":         {{0xFF00, 0xFFEF}},
	"Specials":                           {{0xFEFF, 0xFEFF}, {0xFFF0, 0xFFFD}},
}

var xmlWhitespaceRanges = []runeRange{
	{0x0009, 0x000A},
	{0x000D, 0x000D},
	{0x0020, 0x0020},
}

var xmlNameStartRanges = []runeRange{
	{0x003A, 0x003A},
	{0x0041, 0x005A},
	{0x005F, 0x005F},
	{0x0061, 0x007A},
	{0x00C0, 0x00D6},
	{0x00D8, 0x00F6},
	{0x00F8, 0x02FF},
	{0x0370, 0x037D},
	{0x037F, 0x1FFF},
	{0x200C, 0x200D},
	{0x2070, 0x218F},
	{0x2C00, 0x2FEF},
	{0x3001, 0xD7FF},
	{0xF900, 0xFDCF},
	{0xFDF0, 0xFFFD},
	{0x10000, 0xEFFFF},
}

var xmlNameCharRanges = []runeRange{
	{0x002D, 0x002E},
	{0x0030, 0x003A},
	{0x0041, 0x005A},
	{0x005F, 0x005F},
	{0x0061, 0x007A},
	{0x00B7, 0x00B7},
	{0x00C0, 0x00D6},
	{0x00D8, 0x00F6},
	{0x00F8, 0x036F},
	{0x0370, 0x037D},
	{0x037F, 0x1FFF},
	{0x200C, 0x200D},
	{0x203F, 0x2040},
	{0x2070, 0x218F},
	{0x2C00, 0x2FEF},
	{0x3001, 0xD7FF},
	{0xF900, 0xFDCF},
	{0xFDF0, 0xFFFD},
	{0x10000, 0xEFFFF},
}

func nativeEscapeReplacement(runes []rune, slash int, inClass bool) (string, int, bool) {
	if slash+1 >= len(runes) {
		return "", slash, false
	}
	switch runes[slash+1] {
	case 'd':
		return categoryClassExpr([]string{"Nd"}, false, inClass), slash + 1, true
	case 'D':
		return categoryClassExpr([]string{"Nd"}, true, inClass), slash + 1, true
	case 's':
		return regexpClassExpr(xmlWhitespaceRanges, inClass), slash + 1, true
	case 'S':
		return regexpClassExpr(complementRanges(xmlWhitespaceRanges), inClass), slash + 1, true
	case 'w':
		return categoryClassExpr([]string{"L", "M", "N", "S"}, false, inClass), slash + 1, true
	case 'W':
		return categoryClassExpr([]string{"P", "Z", "C"}, false, inClass), slash + 1, true
	case 'i':
		return regexpClassExpr(xmlNameStartRanges, inClass), slash + 1, true
	case 'I':
		return regexpClassExpr(complementRanges(xmlNameStartRanges), inClass), slash + 1, true
	case 'c':
		return regexpClassExpr(xmlNameCharRanges, inClass), slash + 1, true
	case 'C':
		return regexpClassExpr(complementRanges(xmlNameCharRanges), inClass), slash + 1, true
	}
	if slash+3 >= len(runes) || (runes[slash+1] != 'p' && runes[slash+1] != 'P') || runes[slash+2] != '{' {
		return "", slash, false
	}
	end := -1
	for i := slash + 3; i < len(runes); i++ {
		if runes[i] == '}' {
			end = i
			break
		}
	}
	if end == -1 {
		return "", slash, false
	}
	ranges, ok := unicodeBlockClassRanges(string(runes[slash+3:end]), runes[slash+1] == 'P')
	if !ok {
		return "", slash, false
	}
	return regexpClassExpr(ranges, inClass), end, true
}

func unicodeBlockClassRanges(category string, complement bool) ([]runeRange, bool) {
	if !strings.HasPrefix(category, "Is") {
		return nil, false
	}
	ranges, ok := unicodeBlockRanges[category[len("Is"):]]
	if !ok {
		return nil, false
	}
	if !complement {
		return ranges, true
	}
	out := make([]runeRange, 0, len(ranges)+1)
	next := 0
	for _, r := range ranges {
		lo := int(r.lo)
		hi := int(r.hi)
		if lo > next {
			out = append(out, runeRange{lo: rune(next), hi: rune(lo - 1)})
		}
		if hi+1 > next {
			next = hi + 1
		}
	}
	if next <= utf8.MaxRune {
		out = append(out, runeRange{lo: rune(next), hi: utf8.MaxRune})
	}
	return out, true
}

func regexpClassExpr(ranges []runeRange, inClass bool) string {
	ranges = normalizeRanges(ranges)
	if len(ranges) == 0 && !inClass {
		return `[^\x{0}-\x{10FFFF}]`
	}
	var out strings.Builder
	if !inClass {
		out.WriteByte('[')
	}
	for _, r := range ranges {
		writeRegexpClassRune(&out, r.lo)
		if r.hi > r.lo {
			out.WriteByte('-')
			writeRegexpClassRune(&out, r.hi)
		}
	}
	if !inClass {
		out.WriteByte(']')
	}
	return out.String()
}

func categoryClassExpr(categories []string, complement bool, inClass bool) string {
	var out strings.Builder
	if !inClass {
		out.WriteByte('[')
	}
	for _, category := range categories {
		if complement {
			out.WriteString(`\P{`)
		} else {
			out.WriteString(`\p{`)
		}
		out.WriteString(category)
		out.WriteByte('}')
	}
	if !inClass {
		out.WriteByte(']')
	}
	return out.String()
}

func complementRanges(ranges []runeRange) []runeRange {
	ranges = normalizeRanges(ranges)
	out := make([]runeRange, 0, len(ranges)+1)
	next := 0
	for _, r := range ranges {
		lo := int(r.lo)
		hi := int(r.hi)
		if lo > next {
			out = append(out, runeRange{lo: rune(next), hi: rune(lo - 1)})
		}
		if hi+1 > next {
			next = hi + 1
		}
	}
	if next <= utf8.MaxRune {
		out = append(out, runeRange{lo: rune(next), hi: utf8.MaxRune})
	}
	return out
}

func normalizeRanges(ranges []runeRange) []runeRange {
	if len(ranges) == 0 {
		return nil
	}
	out := append([]runeRange(nil), ranges...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].lo == out[j].lo {
			return out[i].hi < out[j].hi
		}
		return out[i].lo < out[j].lo
	})
	merged := out[:0]
	for _, r := range out {
		if r.hi < r.lo {
			continue
		}
		if len(merged) == 0 || int(r.lo) > int(merged[len(merged)-1].hi)+1 {
			merged = append(merged, r)
			continue
		}
		if r.hi > merged[len(merged)-1].hi {
			merged[len(merged)-1].hi = r.hi
		}
	}
	return merged
}

func subtractRanges(base, sub []runeRange) []runeRange {
	base = normalizeRanges(base)
	sub = normalizeRanges(sub)
	if len(base) == 0 || len(sub) == 0 {
		return base
	}
	out := make([]runeRange, 0, len(base))
	subIndex := 0
	for _, b := range base {
		cur := b.lo
		for subIndex < len(sub) && sub[subIndex].hi < cur {
			subIndex++
		}
		for i := subIndex; i < len(sub) && sub[i].lo <= b.hi; i++ {
			if sub[i].lo > cur {
				out = append(out, runeRange{lo: cur, hi: rune(int(sub[i].lo) - 1)})
			}
			if sub[i].hi >= b.hi {
				cur = rune(int(b.hi) + 1)
				break
			}
			cur = rune(int(sub[i].hi) + 1)
		}
		if cur <= b.hi {
			out = append(out, runeRange{lo: cur, hi: b.hi})
		}
	}
	return out
}

func nativeClassReplacement(runes []rune, start int) (string, int, bool) {
	_, _, _, end, ok := findClassSubtraction(runes, start)
	if !ok {
		return "", start, false
	}
	ranges, parsedEnd, ok := parseClassExprRanges(runes, start)
	if !ok || parsedEnd != end {
		return "", end, false
	}
	return regexpClassExpr(ranges, false), end, true
}

func findClassSubtraction(runes []rune, start int) (marker, subStart, subEnd, end int, ok bool) {
	marker = -1
	subStart = -1
	subEnd = -1
	depth := 0
	for i := start; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			i = skipEscape(runes, i)
		case '[':
			depth++
			if depth == 2 && marker != -1 && subStart == -1 {
				subStart = i
			}
		case ']':
			if depth == 2 && marker != -1 && subEnd == -1 {
				subEnd = i
			}
			depth--
			if depth == 0 {
				return marker, subStart, subEnd, i, marker != -1 && subStart != -1 && subEnd != -1
			}
		case '-':
			if depth == 1 && i+1 < len(runes) && runes[i+1] == '[' && marker == -1 {
				marker = i
			}
		}
	}
	return -1, -1, -1, len(runes) - 1, false
}

func parseClassExprRanges(runes []rune, start int) ([]runeRange, int, bool) {
	marker, subStart, subEnd, end, ok := findClassSubtraction(runes, start)
	if ok {
		base, ok := parseClassContentRanges(runes[start+1 : marker])
		if !ok {
			return nil, end, false
		}
		sub, parsedSubEnd, ok := parseClassExprRanges(runes, subStart)
		if !ok || parsedSubEnd != subEnd {
			return nil, end, false
		}
		return subtractRanges(base, sub), end, true
	}
	end, ok = findClassEnd(runes, start)
	if !ok {
		return nil, len(runes) - 1, false
	}
	ranges, ok := parseClassContentRanges(runes[start+1 : end])
	return ranges, end, ok
}

func findClassEnd(runes []rune, start int) (int, bool) {
	depth := 0
	for i := start; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			i = skipEscape(runes, i)
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return len(runes) - 1, false
}

func parseClassContentRanges(runes []rune) ([]runeRange, bool) {
	if len(runes) == 0 {
		return nil, false
	}
	negated := false
	if runes[0] == '^' {
		negated = true
		runes = runes[1:]
		if len(runes) == 0 {
			return nil, false
		}
	}
	ranges := make([]runeRange, 0, len(runes))
	for i := 0; i < len(runes); {
		start, ok := parseClassAtom(runes, i)
		if !ok {
			return nil, false
		}
		if start.next < len(runes) && runes[start.next] == '-' && start.next+1 < len(runes) {
			end, ok := parseClassAtom(runes, start.next+1)
			if !ok || !start.singleOK || !end.singleOK || start.single > end.single {
				return nil, false
			}
			ranges = append(ranges, runeRange{lo: start.single, hi: end.single})
			i = end.next
			continue
		}
		ranges = append(ranges, start.ranges...)
		i = start.next
	}
	ranges = normalizeRanges(ranges)
	if negated {
		return complementRanges(ranges), true
	}
	return ranges, true
}

type classAtom struct {
	ranges   []runeRange
	single   rune
	singleOK bool
	next     int
}

func parseClassAtom(runes []rune, start int) (classAtom, bool) {
	if start >= len(runes) {
		return classAtom{}, false
	}
	r := runes[start]
	if r == '[' || r == ']' {
		return classAtom{}, false
	}
	if r == '\\' {
		return parseClassEscapeAtom(runes, start)
	}
	return classAtom{
		ranges:   []runeRange{{lo: r, hi: r}},
		single:   r,
		singleOK: true,
		next:     start + 1,
	}, true
}

func parseClassEscapeAtom(runes []rune, slash int) (classAtom, bool) {
	if slash+1 >= len(runes) {
		return classAtom{}, false
	}
	literal := func(r rune) (classAtom, bool) {
		return classAtom{
			ranges:   []runeRange{{lo: r, hi: r}},
			single:   r,
			singleOK: true,
			next:     slash + 2,
		}, true
	}
	switch runes[slash+1] {
	case 'd':
		ranges, ok := unicodeCategoryRanges([]string{"Nd"}, false)
		return classAtom{ranges: ranges, next: slash + 2}, ok
	case 'D':
		ranges, ok := unicodeCategoryRanges([]string{"Nd"}, true)
		return classAtom{ranges: ranges, next: slash + 2}, ok
	case 's':
		return classAtom{ranges: xmlWhitespaceRanges, next: slash + 2}, true
	case 'S':
		return classAtom{ranges: complementRanges(xmlWhitespaceRanges), next: slash + 2}, true
	case 'w':
		ranges, ok := unicodeCategoryRanges([]string{"L", "M", "N", "S"}, false)
		return classAtom{ranges: ranges, next: slash + 2}, ok
	case 'W':
		ranges, ok := unicodeCategoryRanges([]string{"L", "M", "N", "S"}, true)
		return classAtom{ranges: ranges, next: slash + 2}, ok
	case 'i':
		return classAtom{ranges: xmlNameStartRanges, next: slash + 2}, true
	case 'I':
		return classAtom{ranges: complementRanges(xmlNameStartRanges), next: slash + 2}, true
	case 'c':
		return classAtom{ranges: xmlNameCharRanges, next: slash + 2}, true
	case 'C':
		return classAtom{ranges: complementRanges(xmlNameCharRanges), next: slash + 2}, true
	case 'n':
		return literal('\n')
	case 'r':
		return literal('\r')
	case 't':
		return literal('\t')
	case '.', '\\', '?', '*', '+', '{', '}', '(', ')', '|', '[', ']', '^', '$', '-':
		return literal(runes[slash+1])
	case 'p', 'P':
		if slash+2 >= len(runes) || runes[slash+2] != '{' {
			return classAtom{}, false
		}
		end := -1
		for i := slash + 3; i < len(runes); i++ {
			if runes[i] == '}' {
				end = i
				break
			}
		}
		if end == -1 || end == slash+3 {
			return classAtom{}, false
		}
		ranges, ok := unicodeClassRanges(string(runes[slash+3:end]), runes[slash+1] == 'P')
		return classAtom{ranges: ranges, next: end + 1}, ok
	default:
		return classAtom{}, false
	}
}

func unicodeCategoryRanges(categories []string, complement bool) ([]runeRange, bool) {
	ranges := make([]runeRange, 0, len(categories))
	for _, category := range categories {
		table, ok := unicode.Categories[category]
		if !ok {
			return nil, false
		}
		ranges = append(ranges, unicodeRangeTableRanges(table)...)
	}
	ranges = normalizeRanges(ranges)
	if complement {
		return complementRanges(ranges), true
	}
	return ranges, true
}

func unicodeClassRanges(category string, complement bool) ([]runeRange, bool) {
	if strings.HasPrefix(category, "Is") {
		return unicodeBlockClassRanges(category, complement)
	}
	table, ok := unicode.Categories[category]
	if !ok {
		return nil, false
	}
	ranges := unicodeRangeTableRanges(table)
	if complement {
		return complementRanges(ranges), true
	}
	return ranges, true
}

func unicodeRangeTableRanges(table *unicode.RangeTable) []runeRange {
	var ranges []runeRange
	for _, r := range table.R16 {
		ranges = appendStridedRange(ranges, rune(r.Lo), rune(r.Hi), rune(r.Stride))
	}
	for _, r := range table.R32 {
		ranges = appendStridedRange(ranges, rune(r.Lo), rune(r.Hi), rune(r.Stride))
	}
	return normalizeRanges(ranges)
}

func appendStridedRange(ranges []runeRange, lo, hi, stride rune) []runeRange {
	if stride <= 1 {
		return append(ranges, runeRange{lo: lo, hi: hi})
	}
	for r := lo; r <= hi; r += stride {
		ranges = append(ranges, runeRange{lo: r, hi: r})
	}
	return ranges
}

func writeRegexpClassRune(out *strings.Builder, r rune) {
	fmt.Fprintf(out, `\x{%X}`, r)
}

func unsupportedNativeClassSyntax(runes []rune, start int) (int, string) {
	for i := start + 1; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			i = skipEscape(runes, i)
		case '[':
			if i+1 < len(runes) && runes[i+1] == ':' {
				if end, ok := skipPOSIXCharacterClass(runes, i); ok {
					i = end
					continue
				}
			}
			if _, end, ok := nativeClassReplacement(runes, start); ok {
				return end, ""
			}
			return i, "unsupported or malformed character class syntax"
		case ']':
			return i, ""
		}
	}
	return len(runes) - 1, ""
}

func skipEscape(runes []rune, slash int) int {
	if slash+1 >= len(runes) {
		return slash
	}
	if (runes[slash+1] == 'p' || runes[slash+1] == 'P') && slash+2 < len(runes) && runes[slash+2] == '{' {
		for i := slash + 3; i < len(runes); i++ {
			if runes[i] == '}' {
				return i
			}
		}
	}
	return slash + 1
}

func skipPOSIXCharacterClass(runes []rune, start int) (int, bool) {
	for i := start + 2; i+1 < len(runes); i++ {
		if runes[i] == ':' && runes[i+1] == ']' {
			return i + 1, true
		}
	}
	return start, false
}
