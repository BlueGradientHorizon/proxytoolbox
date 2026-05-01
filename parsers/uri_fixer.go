package parsers

import (
	"errors"
	"html"
	"net/url"
	"strconv"
	"strings"
)

func tryFixURI(uri string) (string, error) {
	// Don't even try to understand what the fuck is going on here, I don't know either

	// Let's take an example config uri:
	// trojan://8r<[9'l6hAO#8ZQi&==@@46.8.228.74:2053?host=Koma-YT.PAGeS.Dev&path=/trTelegram🇨🇳 @WangCai2%GG&security=tls&sni=Koma-YT.PAGeS.Dev&type=ws&note=something#%1F7410 | 🇫🇮 Finland | TROJAN | 📺 YT | TG: @YoutubeUnBlockRu

	// Fix №1: trim string

	uri = strings.TrimSpace(uri)
	if uri == "" {
		return "", errors.New("empty URI")
	}

	// The string however is usually already trimmed by ParseConfig

	// Fix №2: remove spaces in url (before `#`)

	// beforeRemarkIndex := strings.Index(uri, "#")
	// if beforeRemarkIndex == -1 {
	// 	beforeRemarkIndex = len(uri)
	// }
	// beforeRemark := uri[:beforeRemarkIndex]
	// var afterRemark string
	// if beforeRemarkIndex < len(uri) {
	// 	afterRemark = uri[beforeRemarkIndex:]
	// }
	// beforeRemark = strings.ReplaceAll(beforeRemark, " ", "")
	// uri = beforeRemark + afterRemark

	// TODO: weird trojan uris may contain shebang in userinfo like in the example so this won't work

	// Fix №3: clean malformed percent encoding

	uri = cleanMalformedPercentEncoding(uri)

	// trojan://8r<[9'l6hAO#8ZQi&==@@46.8.228.74:2053?host=Koma-YT.PAGeS.Dev&path=/trTelegram🇨🇳 @WangCai2&security=tls&sni=Koma-YT.PAGeS.Dev&type=ws&note=something#%1F7410 | 🇫🇮 Finland | TROJAN | 📺 YT | TG: @YoutubeUnBlockRu
	// Invalid %GG is removed

	// Fix №4: remove control characters

	uri = removePercentEncodedControlCharacters(uri)

	// trojan://8r<[9'l6hAO#8ZQi&==@@46.8.228.74:2053?host=Koma-YT.PAGeS.Dev&path=/trTelegram🇨🇳 @WangCai2&security=tls&sni=Koma-YT.PAGeS.Dev&type=ws&note=something#7410 | 🇫🇮 Finland | TROJAN | 📺 YT | TG: @YoutubeUnBlockRu
	// Valid but unacceptable %1F is removed

	// Fix #5: unescape percent encoding
	// Example not changed

	uri, _ = url.PathUnescape(uri)

	// Fix #6: escape all ampersands before unescaping to prevent some query params like "&note=" being unescaped to "¬e=" thus breaking params parsing later

	uri = escapeAmpersands(uri)
	// trojan://8r<[9'l6hAO#8ZQi&amp;==@@46.8.228.74:2053?host=Koma-YT.PAGeS.Dev&amp;path=/trTelegram🇨🇳 @WangCai2&amp;security=tls&amp;sni=Koma-YT.PAGeS.Dev&amp;type=ws&amp;note=something#7410 | 🇫🇮 Finland | TROJAN | 📺 YT | TG: @YoutubeUnBlockRu

	// Fix №7: unescape string (html named entities like "&amp;")

	uri = html.UnescapeString(uri)

	// trojan://8r<[9'l6hAO#8ZQi&==@@46.8.228.74:2053?host=Koma-YT.PAGeS.Dev&path=/trTelegram🇨🇳 @WangCai2&security=tls&sni=Koma-YT.PAGeS.Dev&type=ws&note=something#7410 | 🇫🇮 Finland | TROJAN | 📺 YT | TG: @YoutubeUnBlockRu

	// Fix #8: escape user part

	schemeSplit := strings.SplitN(uri, "://", 2)
	if len(schemeSplit) != 2 {
		return uri, nil
	}

	// trojan
	scheme := schemeSplit[0]

	// 8r<[9'l6hAO#8ZQi&==@@46.8.228.74:2053?host=Koma-YT.PAGeS.Dev&path=/trTelegram🇨🇳 @WangCai2&security=tls&sni=Koma-YT.PAGeS.Dev&type=ws&note=something#7410 | 🇫🇮 Finland | TROJAN | 📺 YT | TG: @YoutubeUnBlockRu
	rest := schemeSplit[1]

	userSplitIndex := -1
	authorityEnd := -1

	// Scan forward to find the true authority boundary.
	// In proxy URIs, userinfo can contain '#', but the hostport segment
	// (immediately following the separator '@') must consist of valid host characters.
	for i := 0; i < len(rest); i++ {
		if rest[i] == '@' {
			// Check if the segment following this '@' is a valid hostport.
			j := i + 1
			for j < len(rest) {
				c := rest[j]
				// Valid hostport characters: alphanumeric, dots, hyphens, colons, or brackets.
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
					c == '.' || c == '-' || c == ':' || c == '[' || c == ']' {
					j++
				} else {
					break
				}
			}

			// If the segment is non-empty, we have a candidate for the authority section.
			if j > i+1 {
				userSplitIndex = i
				authorityEnd = j

				// If this hostport segment is followed by a path (/), query (?),
				// or fragment (#), the authority section is definitively over.
				if j < len(rest) && (rest[j] == '/' || rest[j] == '?' || rest[j] == '#') {
					break
				}
			}
		} else if rest[i] == '/' || rest[i] == '?' {
			// If we hit the path or query start before finding any valid authority,
			// the authority section (if any) is over.
			break
		}
	}

	// If no valid authority structure was found, return the URI as is.
	if userSplitIndex == -1 {
		return uri, nil
	}

	user := rest[:userSplitIndex] // 8r<[9'l6hAO#8Z Qi&==@
	addr := rest[userSplitIndex+1 : authorityEnd] // 46.8.228.74:2053

	// Try unescaping user part again just in a case (sounds weird, I know)
	userUnescaped, err := url.PathUnescape(user)
	if err == nil {
		user = userUnescaped
	}

	// Final user part escape
	// QueryEscape is more aggressive, so using it instead of Path-
	userEscaped := url.QueryEscape(user)

	// url.QueryEscape escapes spaces to + (RFC 3986) but since very weirdly crafted links could contain it in user part, we'll do it manually
	userEscaped = strings.ReplaceAll(userEscaped, "+", "%20")

	afterAuthority := rest[authorityEnd:] // In example, starts with ?host=
	uri = scheme + "://" + userEscaped + "@" + addr + afterAuthority

	// trojan://8r%3C%5B9%27l6hAO%238ZQi&==%40@46.8.228.74:2053?host=Koma-YT.PAGeS.Dev&path=/trTelegram🇨🇳 @WangCai2&security=tls&sni=Koma-YT.PAGeS.Dev&type=ws&note=something#7410 | 🇫🇮 Finland | TROJAN | 📺 YT | TG: @YoutubeUnBlockRu

	return uri, nil
}

func cleanMalformedPercentEncoding(input string) string {
	var builder strings.Builder
	builder.Grow(len(input))

	for i := 0; i < len(input); i++ {
		if input[i] == '%' {
			if i+2 < len(input) && isHex(input[i+1]) && isHex(input[i+2]) {
				builder.WriteString(input[i : i+3])
				i += 2
			} else {
				// Intentionally skip sequences like %GG for safety
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
