package persistence

import "strings"

// qualified prefixes each column in a comma-separated list with a table alias,
// for RETURNING clauses on UPDATE … FROM.
func qualified(cols, alias string) string {
	parts := strings.Split(cols, ", ")
	for i, c := range parts {
		parts[i] = alias + "." + c
	}
	return strings.Join(parts, ", ")
}
