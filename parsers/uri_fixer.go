package parsers

import (
	"errors"
	"html"
	"net/url"
	"strconv"
	"strings"
)

func tryFixURI(uri string) (string, error) {
	// Let's take an example config uri:
	// trojan://8r<[9'l6hAO#8ZQi&==@@46.8.228.74:2053?host=Koma-YT.PAGeS.Dev&path=/trTelegram🇨🇳 @WangCai2%GG&security=tls&sni=Koma-YT.PAGeS.Dev&type=ws&note=something#%1F7410 | 🇫🇮 Finland | TROJAN | 📺 YT | TG: @YoutubeUnBlockRu

	// Fix №1: trim string

	uri = strings.TrimSpace(uri)
	if uri == "" {
		return "", errors.New("empty URI")
	}

	// The string should already be trimmed by ParseConfig

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
	authorityEnd := len(rest) // 220

	// Find the end of the authority section (first / or ?)
	// Since we haven't unescaped the URI yet, literal ? or / marks the true start of path/query
	firstSlashOrQuestion := len(rest)
	for i, c := range rest {
		// For links like ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTprTEFvUFNueWdtWXJaLUI1am8zQXRrei1BY2VWaHQtZA@83.166.255.72:1080#🇷🇺 [AS47764] LLC VK #1788.%20%F0%9F%87%B7%F0%9F%87%BA%20SS%20%7C%20CIDR%3A%20VK%20%7C%20TG%3A%20%40wlrustg this won't work because there's shebang only, but if I add || c == '#' this will break parsing of trojan links like in example. What to do? I want this function to remain completely protocol-agnostic.
		if c == '/' || c == '?' {
			firstSlashOrQuestion = i // 37, the question mark before host=
			break
		}
	}

	// Find the true '@' that separates userinfo from host.
	// We scan from right to left, but strictly before the path/query section.

	// len of 8r<[9'l6hAO#8ZQi&==@@46.8.228.74:2053 is 37

	// From i=36 to i=0
	for i := firstSlashOrQuestion - 1; i >= 0; i-- {
		if rest[i] == '@' { // The index of last @ is 20
			// The hostport should start here and end at the first / or ? or #
			end := firstSlashOrQuestion

			// TODO: is this needed? Could shebang be before / or ?
			for j := i + 1; j < firstSlashOrQuestion; j++ {
				if rest[j] == '#' {
					end = j
					break
				}
			}

			hostport := rest[i+1 : end] // 21:36 46.8.228.74:2053

			// A valid hostport in proxy URIs must strictly contain valid domain/IP characters
			for _, c := range hostport {
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == ':' || c == '[' || c == ']' {
					continue
				} else {
					return "", errors.New("invalid hostname")
				}
			}

			userSplitIndex = i
			authorityEnd = end
			break
		}
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
