package main

import (
	"fmt"
	"strings"
)

// splitArgs splits a command line into argv using POSIX-shell quoting rules
// only: single quotes are literal, double quotes keep spaces and allow \" and
// \\ escapes, and a backslash outside quotes escapes the next character.
// No expansion, globbing, pipes, or redirects — nothing runs through a shell.
func splitArgs(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inArg := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, fmt.Errorf("unterminated single quote in %q", s)
			}
			cur.WriteString(s[i+1 : i+1+j])
			i += j + 1
			inArg = true
		case c == '"':
			i++
			for ; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
					i++
				}
				cur.WriteByte(s[i])
			}
			if i >= len(s) {
				return nil, fmt.Errorf("unterminated double quote in %q", s)
			}
			inArg = true
		case c == '\\':
			if i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			}
			inArg = true
		case c == ' ' || c == '\t' || c == '\n':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteByte(c)
			inArg = true
		}
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}
