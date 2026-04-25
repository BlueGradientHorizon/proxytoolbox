package utils

import (
	"errors"
	"html"
	"net/url"
	"strconv"
	"strings"
)

func TryFixURI(uri string) (string, error) {
	// Fix №1: trim string
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return "", errors.New("empty URI")
	}

	// Fix №2: remove spaces in url (before `#`)
	beforeRemarkIndex := strings.Index(uri, "#")
	if beforeRemarkIndex == -1 {
		beforeRemarkIndex = len(uri)
	}
	beforeRemark := uri[:beforeRemarkIndex]
	var afterRemark string
	if beforeRemarkIndex == len(uri) {
		afterRemark = ""
	} else {
		afterRemark = uri[beforeRemarkIndex+1:]
	}
	beforeRemark = strings.ReplaceAll(beforeRemark, " ", "")
	uri = beforeRemark + "#" + afterRemark

	// Fix №3: clean malformed percent encoding
	uri = cleanMalformedPercentEncoding(uri)

	// Fix №4: remove control characterss
	uri = removePercentEncodedControlCharacters(uri)

	// Fix №5: unescape uri (percent encoding)
	uri, err := url.QueryUnescape(uri)
	if err != nil {
		// Since the fix №3 is applied (and is correctly implemented) it should never return err
		return "", err
	}

	// Fix №6: clean malformed percent encoding again
	uri = cleanMalformedPercentEncoding(uri)

	// Fix #7: escape all ampersands before unescaping to prevent some query params like "&note=" being unescaped to "¬e=" thus breaking params parsing later
	uri = escapeAmpersands(uri)

	// Fix №8: unescape string (html named entities like "&amp;")
	uri = html.UnescapeString(uri)

	// Fix #9: escape user part
	schemeSplit := strings.SplitN(uri, "://", 2)
	if len(schemeSplit) != 2 {
		return "", errors.New("failed to split URI by scheme")
	}
	querySplit := strings.Split(schemeSplit[1], "?")
	if len(querySplit) == 2 {
		userSplitIndex := strings.LastIndex(querySplit[0], "@")
		if userSplitIndex != -1 {
			user := querySplit[0][0:userSplitIndex]
			addr := querySplit[0][userSplitIndex+1:]
			userEscaped := url.QueryEscape(user)
			uri = schemeSplit[0] + "://" + userEscaped + "@" + addr + "?" + querySplit[1]
		}
	}

	return uri, nil
}

func cleanMalformedPercentEncoding(input string) string {
	var builder strings.Builder
	builder.Grow(len(input))

	for i := 0; i < len(input); i++ {
		if input[i] == '%' {
			if i+2 < len(input) && isHex(input[i+1]) && isHex(input[i+2]) {
				builder.WriteByte(input[i])
				builder.WriteByte(input[i+1])
				builder.WriteByte(input[i+2])
				i += 2
			}
		} else {
			builder.WriteByte(input[i])
		}
	}

	return builder.String()
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'f') ||
		(c >= 'A' && c <= 'F')
}

func removePercentEncodedControlCharacters(input string) string {
	var b strings.Builder
	b.Grow(len(input))

	for i := 0; i < len(input); i++ {
		if input[i] == '%' && i+2 < len(input) {
			hexStr := input[i+1 : i+3]
			val, err := strconv.ParseUint(hexStr, 16, 8)

			if err == nil {
				if (val <= 0x1F) || val == 0x7F {
					i += 2
					continue
				}
			}
		}
		b.WriteByte(input[i])
	}

	return b.String()
}

func escapeAmpersands(input string) string {
	var builder strings.Builder
	builder.Grow(len(input))

	for i := 0; i < len(input); i++ {
		if input[i] == '&' {
			if i+4 < len(input) && input[i+1:i+5] == "amp;" {
				builder.WriteByte('&')
			} else {
				builder.WriteString("&amp;")
			}
		} else {
			builder.WriteByte(input[i])
		}
	}
	return builder.String()
}
