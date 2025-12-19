package bot

import (
	"testing"

	"github.com/mtibben/confusables"
)

func TestStripZeroWidthChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no zero-width chars",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "zero-width space",
			input:    "hello\u200Bworld",
			expected: "helloworld",
		},
		{
			name:     "zero-width non-joiner",
			input:    "test\u200Ctest",
			expected: "testtest",
		},
		{
			name:     "zero-width joiner",
			input:    "test\u200Dtest",
			expected: "testtest",
		},
		{
			name:     "zero-width no-break space",
			input:    "test\uFEFFtest",
			expected: "testtest",
		},
		{
			name:     "multiple zero-width chars",
			input:    "a\u200Bb\u200Cc\u200Dd\uFEFFe",
			expected: "abcde",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only zero-width chars",
			input:    "\u200B\u200C\u200D\uFEFF",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripZeroWidthChars(tt.input)
			if result != tt.expected {
				t.Errorf("stripZeroWidthChars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBuildSkeletonMatcher(t *testing.T) {
	tests := []struct {
		name  string
		words []string
		wantNil bool
	}{
		{
			name:    "empty words",
			words:   []string{},
			wantNil: true,
		},
		{
			name:    "nil words",
			words:   nil,
			wantNil: true,
		},
		{
			name:    "single word",
			words:   []string{"test"},
			wantNil: false,
		},
		{
			name:    "multiple words",
			words:   []string{"test", "hello", "world"},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := buildSkeletonMatcher(tt.words)
			if (matcher == nil) != tt.wantNil {
				t.Errorf("buildSkeletonMatcher(%v) = %v, want nil=%v", tt.words, matcher, tt.wantNil)
			}
		})
	}
}

func TestSkeletonMatching_Homoglyphs(t *testing.T) {
	// Test that Cyrillic characters match their Latin equivalents
	censoredWords := []string{"test", "hello"}
	matcher := buildSkeletonMatcher(censoredWords)

	tests := []struct {
		name    string
		text    string
		shouldMatch bool
	}{
		{
			name:        "exact match",
			text:        "test",
			shouldMatch: true,
		},
		{
			name:        "Cyrillic e instead of Latin e",
			text:        "tеst", // Cyrillic е (U+0435)
			shouldMatch: true,
		},
		{
			name:        "Cyrillic o instead of Latin o",
			text:        "hellо", // Cyrillic о (U+043E)
			shouldMatch: true,
		},
		{
			name:        "no match",
			text:        "goodbye",
			shouldMatch: false,
		},
		{
			name:        "partial match in word",
			text:        "testing",
			shouldMatch: true, // "test" is a substring
		},
		{
			name:        "word in sentence",
			text:        "this is a test",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := stripZeroWidthChars(tt.text)
			skeleton := confusables.Skeleton(cleaned)
			matches := matcher.Match([]byte(skeleton))
			matched := len(matches) > 0

			if matched != tt.shouldMatch {
				t.Errorf("text %q (skeleton: %q) matched=%v, want %v", tt.text, skeleton, matched, tt.shouldMatch)
			}
		})
	}
}

func TestSkeletonMatching_ZeroWidthChars(t *testing.T) {
	censoredWords := []string{"test"}
	matcher := buildSkeletonMatcher(censoredWords)

	tests := []struct {
		name    string
		text    string
		shouldMatch bool
	}{
		{
			name:        "zero-width space evasion",
			text:        "t\u200Best",
			shouldMatch: true,
		},
		{
			name:        "zero-width non-joiner evasion",
			text:        "t\u200Cest",
			shouldMatch: true,
		},
		{
			name:        "zero-width joiner evasion",
			text:        "t\u200Dest",
			shouldMatch: true,
		},
		{
			name:        "multiple zero-width chars",
			text:        "t\u200Be\u200Cs\u200Dt",
			shouldMatch: true,
		},
		{
			name:        "normal text",
			text:        "test",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := stripZeroWidthChars(tt.text)
			skeleton := confusables.Skeleton(cleaned)
			matches := matcher.Match([]byte(skeleton))
			matched := len(matches) > 0

			if matched != tt.shouldMatch {
				t.Errorf("text %q (cleaned: %q, skeleton: %q) matched=%v, want %v", 
					tt.text, cleaned, skeleton, matched, tt.shouldMatch)
			}
		})
	}
}

func TestSkeletonMatching_CaseInsensitive(t *testing.T) {
	// Note: TR-39 skeletons preserve case, so for case-insensitive matching,
	// we need to convert to lowercase before skeleton generation (which the actual
	// implementation does via strings.ToLower in processConfession).
	// This test verifies that lowercase matching works correctly.
	censoredWords := []string{"test", "hello"}
	matcher := buildSkeletonMatcher(censoredWords)

	tests := []struct {
		name    string
		text    string
		shouldMatch bool
	}{
		{
			name:        "lowercase",
			text:        "test",
			shouldMatch: true,
		},
		{
			name:        "lowercase in sentence",
			text:        "this is a test",
			shouldMatch: true,
		},
		{
			name:        "uppercase (would need lowercase conversion in real code)",
			text:        "TEST",
			shouldMatch: false, // Skeletons preserve case, so "TEST" skeleton != "test" skeleton
		},
		{
			name:        "mixed case (would need lowercase conversion in real code)",
			text:        "TeSt",
			shouldMatch: false, // Skeletons preserve case
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := stripZeroWidthChars(tt.text)
			skeleton := confusables.Skeleton(cleaned)
			matches := matcher.Match([]byte(skeleton))
			matched := len(matches) > 0

			if matched != tt.shouldMatch {
				t.Errorf("text %q (skeleton: %q) matched=%v, want %v", tt.text, skeleton, matched, tt.shouldMatch)
			}
		})
	}
}

func TestSkeletonMatching_MultipleWords(t *testing.T) {
	censoredWords := []string{"bad", "word", "evil"}
	matcher := buildSkeletonMatcher(censoredWords)

	tests := []struct {
		name    string
		text    string
		shouldMatch bool
		expectedMatches int
	}{
		{
			name:        "single match",
			text:        "this is bad",
			shouldMatch: true,
			expectedMatches: 1,
		},
		{
			name:        "multiple matches",
			text:        "bad word evil",
			shouldMatch: true,
			expectedMatches: 3,
		},
		{
			name:        "no matches",
			text:        "this is fine",
			shouldMatch: false,
			expectedMatches: 0,
		},
		{
			name:        "homoglyph evasion",
			text:        "this is bаd", // Cyrillic а
			shouldMatch: true,
			expectedMatches: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := stripZeroWidthChars(tt.text)
			skeleton := confusables.Skeleton(cleaned)
			matches := matcher.Match([]byte(skeleton))
			matched := len(matches) > 0

			if matched != tt.shouldMatch {
				t.Errorf("text %q matched=%v, want %v", tt.text, matched, tt.shouldMatch)
			}
			if len(matches) != tt.expectedMatches {
				t.Errorf("text %q matched %d times, want %d", tt.text, len(matches), tt.expectedMatches)
			}
		})
	}
}

func TestSkeletonMatching_EdgeCases(t *testing.T) {
	censoredWords := []string{"test"}
	matcher := buildSkeletonMatcher(censoredWords)

	tests := []struct {
		name    string
		text    string
		shouldMatch bool
	}{
		{
			name:        "empty string",
			text:        "",
			shouldMatch: false,
		},
		{
			name:        "only spaces",
			text:        "   ",
			shouldMatch: false,
		},
		{
			name:        "word with punctuation",
			text:        "test!",
			shouldMatch: true,
		},
		{
			name:        "word with numbers",
			text:        "test123",
			shouldMatch: true,
		},
		{
			name:        "similar but different word",
			text:        "best", // different from "test"
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := stripZeroWidthChars(tt.text)
			skeleton := confusables.Skeleton(cleaned)
			matches := matcher.Match([]byte(skeleton))
			matched := len(matches) > 0

			if matched != tt.shouldMatch {
				t.Errorf("text %q (skeleton: %q) matched=%v, want %v", tt.text, skeleton, matched, tt.shouldMatch)
			}
		})
	}
}

func TestSkeletonMatching_RealWorldEvasion(t *testing.T) {
	// Test realistic evasion attempts
	censoredWords := []string{"spam", "scam"}
	matcher := buildSkeletonMatcher(censoredWords)

	tests := []struct {
		name    string
		text    string
		shouldMatch bool
		description string
	}{
		{
			name:        "Cyrillic spam",
			text:        "spаm", // Cyrillic а
			shouldMatch: true,
			description: "Cyrillic 'а' instead of Latin 'a'",
		},
		{
			name:        "zero-width in spam",
			text:        "sp\u200Bam",
			shouldMatch: true,
			description: "Zero-width space in middle",
		},
		{
			name:        "mixed evasion",
			text:        "sc\u200Bаm", // Zero-width + Cyrillic
			shouldMatch: true,
			description: "Combined evasion techniques",
		},
		{
			name:        "legitimate word",
			text:        "I like spam and eggs",
			shouldMatch: true,
			description: "Normal usage",
		},
		{
			name:        "no evasion needed",
			text:        "this is fine",
			shouldMatch: false,
			description: "Clean text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := stripZeroWidthChars(tt.text)
			skeleton := confusables.Skeleton(cleaned)
			matches := matcher.Match([]byte(skeleton))
			matched := len(matches) > 0

			if matched != tt.shouldMatch {
				t.Errorf("%s: text %q (cleaned: %q, skeleton: %q) matched=%v, want %v", 
					tt.description, tt.text, cleaned, skeleton, matched, tt.shouldMatch)
			}
		})
	}
}

func TestBuildSkeletonMatcher_Consistency(t *testing.T) {
	// Test that building a matcher multiple times produces consistent results
	words := []string{"test", "hello"}
	
	matcher1 := buildSkeletonMatcher(words)
	matcher2 := buildSkeletonMatcher(words)

	// Both matchers should match the same text
	testText := "test"
	cleaned := stripZeroWidthChars(testText)
	skeleton := confusables.Skeleton(cleaned)

	matches1 := matcher1.Match([]byte(skeleton))
	matches2 := matcher2.Match([]byte(skeleton))

	if len(matches1) != len(matches2) {
		t.Errorf("matcher1 matched %d times, matcher2 matched %d times", len(matches1), len(matches2))
	}
}

func TestSkeletonMatching_UnicodeNormalization(t *testing.T) {
	// Test that the skeleton algorithm handles Unicode normalization correctly
	censoredWords := []string{"café"}
	matcher := buildSkeletonMatcher(censoredWords)

	tests := []struct {
		name    string
		text    string
		shouldMatch bool
	}{
		{
			name:        "exact match",
			text:        "café",
			shouldMatch: true,
		},
		{
			name:        "in sentence",
			text:        "I went to the café",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := stripZeroWidthChars(tt.text)
			skeleton := confusables.Skeleton(cleaned)
			matches := matcher.Match([]byte(skeleton))
			matched := len(matches) > 0

			if matched != tt.shouldMatch {
				t.Errorf("text %q (skeleton: %q) matched=%v, want %v", tt.text, skeleton, matched, tt.shouldMatch)
			}
		})
	}
}

