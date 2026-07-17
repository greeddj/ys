package app

// natLess reports whether a sorts before b in natural (human) order: runs of
// digits are compared by numeric value, so "staging2" precedes "staging10"
// instead of following it. Non-digit runs are compared byte by byte, matching
// plain lexical order. When two digit runs have the same numeric value, the one
// with fewer leading zeros sorts first, keeping the order total and stable.
func natLess(a, b string) bool {
	for len(a) > 0 && len(b) > 0 {
		if isDigit(a[0]) && isDigit(b[0]) {
			an, bn := digitRun(a), digitRun(b)
			if c := compareDigitRuns(an, bn); c != 0 {
				return c < 0
			}
			a, b = a[len(an):], b[len(bn):]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

// compareDigitRuns orders two all-digit strings by numeric value, breaking ties
// (equal value, different leading zeros) so that fewer digits sort first. It
// returns -1, 0, or 1.
func compareDigitRuns(a, b string) int {
	as, bs := trimLeadingZeros(a), trimLeadingZeros(b)
	switch {
	case len(as) != len(bs):
		return cmpInt(len(as), len(bs))
	case as != bs:
		if as < bs {
			return -1
		}
		return 1
	default:
		return cmpInt(len(a), len(b))
	}
}

// digitRun returns the leading run of ASCII digits in s.
func digitRun(s string) string {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return s[:i]
}

// trimLeadingZeros drops leading '0' bytes but keeps at least one digit so that
// "000" reduces to "0" rather than "".
func trimLeadingZeros(s string) string {
	i := 0
	for i < len(s)-1 && s[i] == '0' {
		i++
	}
	return s[i:]
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
