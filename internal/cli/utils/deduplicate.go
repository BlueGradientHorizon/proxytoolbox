package utils

import "strings"

func DeduplicateConnUris(connUris []string) []string {
	seen := make(map[string]struct{}, len(connUris))
	unique := make([]string, 0, len(connUris))

	for _, connUri := range connUris {
		u := connUri
		// Remove remark
		if strings.Count(u, "#") == 1 {
			u = strings.Split(u, "#")[0]
		}
		if _, exists := seen[connUri]; !exists {
			seen[connUri] = struct{}{}
			unique = append(unique, connUri)
		}
	}

	return unique
}
