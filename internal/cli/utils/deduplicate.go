package utils

import "strings"

// NaiveDeduplicateConfigsUris deduplicates repeated configs connection uris by comparing them with text after shebang (#) symbol excluded. The latter part is often called a remark in proxy clients. Doesn't work with base64-encrypted configs.
func NaiveDeduplicateConfigsUris(connUris []string) []string {
	seen := make(map[string]struct{}, len(connUris))
	unique := make([]string, 0, len(connUris))

	for _, connUri := range connUris {
		u := connUri
		if strings.Count(u, "#") == 1 {
			u = strings.Split(u, "#")[0]
		}
		if _, exists := seen[u]; !exists {
			seen[u] = struct{}{}
			unique = append(unique, connUri)
		}
	}

	return unique
}
