// Package gitignore implements .gitignore-style pattern matching.
// It follows the specification from https://git-scm.com/docs/gitignore
package gitignore

import (
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// Matcher holds patterns and matches paths against them.
type Matcher struct {
	patterns []*pattern
}

// New creates a new Matcher from a list of .gitignore-style patterns.
// Comments (lines starting with #) and empty lines are automatically filtered.
func New(patterns []string) *Matcher {
	m := &Matcher{}
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		m.patterns = append(m.patterns, parsePattern(p))
	}
	return m
}

// Match returns true if the path matches any exclusion pattern.
// The path should be relative to the root directory and use forward slashes.
func (m *Matcher) Match(path string, isDir bool) bool {
	normPath := normalizePath(path)

	for _, p := range m.patterns {
		if p.negate {
			continue // Negation not fully supported yet
		}

		if m.matchPattern(normPath, p, isDir) {
			return true
		}
	}
	return false
}

// matchPattern checks if a path matches a single parsed pattern.
func (m *Matcher) matchPattern(path string, p *pattern, isDir bool) bool {
	// Split path into parts
	pathParts := splitPath(path)

	// Directory-only pattern
	if p.dirOnly && !isDir {
		return false
	}

	// Anchored pattern (starts with /)
	if p.anchored {
		return m.matchAnchored(pathParts, p, isDir)
	}

	// Unanchored pattern - can match at any level
	return m.matchUnanchored(pathParts, p, isDir)
}

// matchAnchored matches a pattern that starts with / (anchored to root).
func (m *Matcher) matchAnchored(pathParts []string, p *pattern, isDir bool) bool {
	patternParts := splitPath(p.pattern)

	// Quick check: must match from start
	return matchParts(pathParts, patternParts, 0, 0, p.dirOnly)
}

// matchUnanchored matches a pattern that doesn't start with /.
// It can match at any level of the path hierarchy.
func (m *Matcher) matchUnanchored(pathParts []string, p *pattern, isDir bool) bool {
	patternParts := splitPath(p.pattern)

	// Try matching from each position in the path
	for i := 0; i <= len(pathParts)-len(patternParts); i++ {
		if matchParts(pathParts, patternParts, i, 0, p.dirOnly) {
			return true
		}
	}

	// Also try matching just the basename for simple patterns
	if len(patternParts) == 1 && !strings.Contains(patternParts[0], "**") {
		for _, part := range pathParts {
			if matchComponent(patternParts[0], part) {
				return true
			}
		}
	}

	return false
}

// pattern represents a parsed .gitignore pattern.
type pattern struct {
	pattern  string
	anchored bool // starts with /
	dirOnly  bool // ends with /
	negate   bool // starts with !
}

// parsePattern parses a single .gitignore pattern string.
func parsePattern(s string) *pattern {
	p := &pattern{}

	// Handle negation
	if strings.HasPrefix(s, "!") {
		p.negate = true
		s = s[1:]
	}

	// Handle anchoring
	if strings.HasPrefix(s, "/") {
		p.anchored = true
		s = s[1:]
	}

	// Handle directory-only
	if strings.HasSuffix(s, "/") {
		p.dirOnly = true
		s = s[:len(s)-1]
	}

	p.pattern = s
	return p
}

// normalizePath converts the path to use forward slashes.
func normalizePath(p string) string {
	if filepath.Separator == '/' {
		return p
	}
	return strings.ReplaceAll(p, string(filepath.Separator), "/")
}

// splitPath splits a path into components, handling leading/trailing slashes.
func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return []string{}
	}
	return strings.Split(p, "/")
}

// matchParts recursively matches pattern parts against path parts.
// Handles ** wildcards that can span multiple directories.
func matchParts(pathParts, patternParts []string, pathIdx, patternIdx int, dirOnly bool) bool {
	// If we've matched all pattern parts, success
	if patternIdx >= len(patternParts) {
		// For directory-only patterns, we must be at a directory
		if dirOnly && pathIdx < len(pathParts) {
			return false
		}
		return true
	}

	// If we've exhausted path parts but still have pattern
	if pathIdx >= len(pathParts) {
		// Only remaining pattern parts can be ** which matches empty
		for i := patternIdx; i < len(patternParts); i++ {
			if patternParts[i] != "**" {
				return false
			}
		}
		return true
	}

	pattern := patternParts[patternIdx]
	path := pathParts[pathIdx]

	// Handle ** - matches zero or more directories
	if pattern == "**" {
		// Try matching zero directories
		if matchParts(pathParts, patternParts, pathIdx, patternIdx+1, dirOnly) {
			return true
		}
		// Try matching one or more directories
		for i := pathIdx; i < len(pathParts); i++ {
			if matchParts(pathParts, patternParts, i+1, patternIdx+1, dirOnly) {
				return true
			}
		}
		return false
	}

	// Normal component matching
	if matchComponent(pattern, path) {
		return matchParts(pathParts, patternParts, pathIdx+1, patternIdx+1, dirOnly)
	}

	return false
}

// matchComponent matches a single pattern component against a path component.
// Supports: *, ?, [abc], [!abc], [a-z]
func matchComponent(pattern, name string) bool {
	// Fast path for exact match
	if pattern == name {
		return true
	}

	// Convert to runes for proper Unicode handling
	patternRunes := []rune(pattern)
	nameRunes := []rune(name)

	return matchRunes(patternRunes, nameRunes)
}

// matchRunes recursively matches runes against each other.
func matchRunes(pattern, name []rune) bool {
	pIdx, nIdx := 0, 0

	for pIdx < len(pattern) || nIdx < len(name) {
		if pIdx >= len(pattern) {
			return false
		}

		pc := pattern[pIdx]

		switch pc {
		case '*':
			// * matches any sequence of characters except /
			if pIdx+1 < len(pattern) && pattern[pIdx+1] == '*' {
				// This is **, should be handled at a higher level
				pIdx++
				continue
			}
			// Try matching zero or more characters
			for i := nIdx; i <= len(name); i++ {
				if matchRunes(pattern[pIdx+1:], name[i:]) {
					return true
				}
			}
			return false

		case '?':
			// ? matches any single character
			if nIdx >= len(name) {
				return false
			}
			pIdx++
			nIdx++

		case '[':
			// Character class: [abc], [!abc], [a-z]
			pIdx++
			negated := false
			if pIdx < len(pattern) && pattern[pIdx] == '!' {
				negated = true
				pIdx++
			}

			// Parse the character class
			var matched bool
			var lastChar rune
			for pIdx < len(pattern) {
				pc := pattern[pIdx]
				pIdx++

				if pc == ']' {
					break
				}

				// Handle range like a-z
				if pIdx < len(pattern) && pattern[pIdx] == '-' {
					pIdx++
					if pIdx < len(pattern) {
						nextChar := pattern[pIdx]
						pIdx++
						if nIdx < len(name) && name[nIdx] >= lastChar && name[nIdx] <= nextChar {
							matched = true
						}
						continue
					}
				}

				lastChar = pc
				if nIdx < len(name) && name[nIdx] == pc {
					matched = true
				}
			}

			if negated == matched {
				return false
			}
			if nIdx >= len(name) {
				return false
			}
			nIdx++

		default:
			// Literal character match
			if nIdx >= len(name) || pc != name[nIdx] {
				return false
			}
			pIdx++
			nIdx++
		}
	}

	return true
}

// --- Shell-style glob matching for fallback ---

// MatchGlob performs simple shell glob matching.
// This is a simpler alternative for patterns without path separators.
func MatchGlob(pattern, name string) bool {
	// Use filepath.Match for basic patterns without **
	if !strings.Contains(pattern, "**") {
		matched, err := filepath.Match(pattern, name)
		if err == nil && matched {
			return true
		}
	}

	// Convert to runes for proper matching
	patternRunes := []rune(pattern)
	nameRunes := []rune(name)

	return matchRunes(patternRunes, nameRunes)
}

// --- Path helper functions ---

// ToSlash converts path separators to forward slashes.
// This is like filepath.ToSlash but handles Windows correctly.
func ToSlash(path string) string {
	if filepath.Separator == '/' {
		return path
	}
	return strings.ReplaceAll(path, string(filepath.Separator), "/")
}

// Base returns the last element of path using forward slash separator.
func Base(path string) string {
	path = ToSlash(path)
	idx := strings.LastIndex(path, "/")
	if idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// Dir returns all but the last element of path.
func Dir(path string) string {
	path = ToSlash(path)
	idx := strings.LastIndex(path, "/")
	if idx >= 0 {
		return path[:idx]
	}
	return "."
}

// --- Platform-specific optimizations ---

func init() {
	// On Windows, we need to be careful with case sensitivity
	if runtime.GOOS == "windows" {
		// Windows is case-insensitive for paths
		// We could add case-folding here if needed
	}
}

// --- Rune length helpers ---

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

func runeAt(s string, idx int) rune {
	for i, r := range s {
		if i == idx {
			return r
		}
	}
	return 0
}
