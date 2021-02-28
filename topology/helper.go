package topology

func isASCIIAlpha(c rune) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z'
}

func isASCIIAlNum(c rune) bool {
	return isASCIIAlpha(c) || '0' <= c && c <= '9'
}

func isASCIIAlNumHyp(c rune) bool {
	return isASCIIAlNum(c) || c == '-'
}

func isValidHostname(s string) bool {
	if len(s) == 0 {
		return false
	}
	if !isASCIIAlpha(rune(s[0])) {
		return false
	}
	for _, c := range s[1 : len(s)-1] {
		if !isASCIIAlNumHyp(c) {
			return false
		}
	}
	return isASCIIAlNum(rune(s[len(s)-1]))
}
