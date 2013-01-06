package tokenize

import (
    "testing"
    //. "github.com/tychofreeman/go-matchers"
    . "matchers"
)

func Tokenize(input string) ([]interface{}) {
    if input == "" {
        return nil
    }
    r := []interface{}{}
    for c := range input {
        if 
    }
    return r
}

func TestEmptyStringYieldsNilList(t *testing.T) {
    AssertThat(t, Tokenize(""), Equals(nil).Or(IsEmpty));
}

func TestBareParensYieldsEmptyList(t *testing.T) {
    AssertThat(t, Tokenize("()"), IsEmpty);
}

func TestIgnoresLeadingAndTrailingSpaces(t *testing.T) {
    AssertThat(t, Tokenize("  () "), IsEmpty)
}

func TestSymbolNamesAddedToList(t *testing.T) {
    AssertThat(t, Tokenize("(a)"), HasExactly("a"))
}
