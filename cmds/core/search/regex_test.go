package search

import (
	"reflect"
	"testing"
)

func TestCompileBreAndExtended(t *testing.T) {
	bre, err := Compile([]string{`\(cat\)\+`}, Options{Basic: true})
	if err != nil {
		t.Fatal(err)
	}
	if !bre.Match([]byte("catcat")) || bre.Match([]byte("dog")) {
		t.Fatal("BRE grouping or GNU repetition operator did not match as expected")
	}

	ere, err := Compile([]string{`(cat|dog)+`}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !ere.Match([]byte("catdog")) {
		t.Fatal("ERE alternation did not match")
	}
}

func TestCompileFixedAndCaseInsensitive(t *testing.T) {
	m, err := Compile([]string{"a.b"}, Options{Fixed: true})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match([]byte("a.b")) || m.Match([]byte("axb")) {
		t.Fatal("fixed string was interpreted as a regular expression")
	}

	m, err = Compile([]string{"münchen"}, Options{Fixed: true, IgnoreCase: true})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match([]byte("MÜNCHEN")) {
		t.Fatal("Unicode case-insensitive fixed match failed")
	}
}

func TestPerlBackreference(t *testing.T) {
	m, err := Compile([]string{`([a-z]+)-\1`}, Options{Perl: true})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match([]byte("item=abc-abc")) || m.Match([]byte("item=abc-def")) {
		t.Fatal("Perl backreference did not match as expected")
	}
}

func TestPerlLookaround(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    [][]int
	}{
		{`(?=needle)needle`, "needle xneedle", [][]int{{0, 6}, {8, 14}}},
		{`(?!bad)(?:bad|good)`, "bad good", [][]int{{4, 8}}},
		{`(?<=prefix)needle`, "prefixneedle xneedle", [][]int{{6, 12}}},
		{`(?<!prefix)needle`, "prefixneedle xneedle", [][]int{{14, 20}}},
	}
	for _, tt := range tests {
		m, err := Compile([]string{tt.pattern}, Options{Perl: true})
		if err != nil {
			t.Fatalf("Compile(%q): %v", tt.pattern, err)
		}
		got := m.FindAllIndex([]byte(tt.input), 0)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("FindAllIndex(%q, %q) = %v, want %v", tt.pattern, tt.input, got, tt.want)
		}
	}
}

func TestFindAllIndexOrdersAndDeduplicatesPatterns(t *testing.T) {
	m, err := Compile([]string{"cat", "dog", "cat"}, Options{Fixed: true})
	if err != nil {
		t.Fatal(err)
	}
	got := m.FindAllIndex([]byte("dog cat cat"), 0)
	want := [][]int{{0, 3}, {4, 7}, {8, 11}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FindAllIndex() = %v, want %v", got, want)
	}
}

func TestFilterWholeWordUsesUnicodeLetters(t *testing.T) {
	data := []byte("chat château")
	matches := [][]int{{0, 4}, {5, 9}, {9, 12}}
	got := FilterWholeWord(data, matches)
	want := [][]int{{0, 4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FilterWholeWord() = %v, want %v", got, want)
	}

}

func TestFixedIndexReturnsMinusOneWhenSearchMisses(t *testing.T) {
	record := []byte("abc")
	if got := FixedIndex(record, []byte("z"), 1, false); got != -1 {
		t.Fatalf("FixedIndex() = %d, want -1", got)
	}
	if got := FixedIndex(record, []byte("bc"), 1, false); got != 1 {
		t.Fatalf("FixedIndex() = %d, want 1", got)
	}
}
