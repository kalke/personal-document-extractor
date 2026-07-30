package normalize

import (
	"regexp"
	"strings"
	"time"
	"unicode"
)

var nonDigits = regexp.MustCompile(`\D+`)

var cpfMasked = regexp.MustCompile(`(?i)\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)

func DigitsOnly(s string) string {
	return nonDigits.ReplaceAllString(s, "")
}

func CPF(s string) string {
	d := DigitsOnly(s)
	if !ValidCPF(d) {
		return ""
	}
	return d
}

func CNPJ(s string) string {
	d := DigitsOnly(s)
	if !ValidCNPJ(d) {
		return ""
	}
	return d
}

func CEP(s string) string {
	d := DigitsOnly(s)
	if len(d) != 8 {
		return ""
	}
	return d
}

func ValidCPF(s string) bool {
	s = DigitsOnly(s)
	if len(s) != 11 || allSameDigits(s) {
		return false
	}
	d := make([]int, 11)
	for i := 0; i < 11; i++ {
		d[i] = int(s[i] - '0')
	}
	sum := 0
	for i := 0; i < 9; i++ {
		sum += d[i] * (10 - i)
	}
	r := (sum * 10) % 11
	if r == 10 {
		r = 0
	}
	if r != d[9] {
		return false
	}
	sum = 0
	for i := 0; i < 10; i++ {
		sum += d[i] * (11 - i)
	}
	r = (sum * 10) % 11
	if r == 10 {
		r = 0
	}
	return r == d[10]
}

func ValidCNPJ(s string) bool {
	s = DigitsOnly(s)
	if len(s) != 14 || allSameDigits(s) {
		return false
	}
	d := make([]int, 14)
	for i := 0; i < 14; i++ {
		d[i] = int(s[i] - '0')
	}
	w1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	w2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 12; i++ {
		sum += d[i] * w1[i]
	}
	r := sum % 11
	dv1 := 0
	if r >= 2 {
		dv1 = 11 - r
	}
	if dv1 != d[12] {
		return false
	}
	sum = 0
	for i := 0; i < 13; i++ {
		sum += d[i] * w2[i]
	}
	r = sum % 11
	dv2 := 0
	if r >= 2 {
		dv2 = 11 - r
	}
	return dv2 == d[13]
}

func FindCPF(text string) string {
	for _, m := range cpfMasked.FindAllString(text, -1) {
		if c := CPF(m); c != "" {
			return c
		}
	}
	return ""
}

func DateToISO(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	s = strings.ReplaceAll(s, ".", "/")
	s = strings.ReplaceAll(s, "-", "/")
	s = collapseSpaces(s)

	layouts := []string{
		"02/01/2006",
		"2/1/2006",
		"02/01/06",
		"2/1/06",
		"2006/01/02",
		"2006/1/2",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			iso := t.Format("2006-01-02")
			return &iso
		}
	}

	if i := strings.IndexFunc(s, unicode.IsSpace); i > 0 {
		return DateToISO(s[:i])
	}
	return nil
}

func allSameDigits(s string) bool {
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			return false
		}
	}
	return true
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
