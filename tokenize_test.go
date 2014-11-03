package glisp

import (
    "testing"

    // Use github version as soon as changes are uploaded.
    //. "github.com/tychofreeman/go-matchers"
    . "matchers"
)

func TestEmptyStringYieldsNilList(t *testing.T) {
    AssertThat(t, TokenizeString(""), IsEmpty);
}

func TestBareParensYieldsEmptyList(t *testing.T) {
    AssertThat(t, TokenizeString("()"), HasExactly(HasExactly()));
}

func TestIgnoresLeadingAndTrailingSpaces(t *testing.T) {
    AssertThat(t, TokenizeString("  () "), HasExactly(HasExactly()))
}

func TestSymbolNamesAddedToList(t *testing.T) {
    AssertThat(t, TokenizeString("(a)"), HasExactly(HasExactly("a")))
}

func TestAddsMultipleSymbolNamesToListInOrder(t *testing.T) {
    AssertThat(t, TokenizeString("(a b c)"), HasExactly(HasExactly("a", "b", "c")))
}

func TestSymbolNamesCanHaveMultipleLetters(t *testing.T) {
    AssertThat(t, TokenizeString("(abc def ghi)"), HasExactly(HasExactly("abc", "def", "ghi")))
}

func TestParensCreatesANestedList(t *testing.T) {
    AssertThat(t, TokenizeString("(a (b))"), HasExactly(HasExactly("a", HasExactly("b"))))
}

func TestNestedListsCanComeInTheMiddleToo(t *testing.T) {
    AssertThat(t, TokenizeString("(a (b) c)"), HasExactly(HasExactly("a", HasExactly("b"), "c")))
}

func TestDeeplyNestedListsWorkToo(t *testing.T) {
    AssertThat(t, TokenizeString("(a (b (c (d (e (f (g (h (i (j (k (l))))))))))) m (n (o (p (q (r))))) s)"), 
        HasExactly(
            HasExactly("a",
                        HasExactly("b", HasExactly("c", HasExactly("d", HasExactly("e", HasExactly("f", HasExactly("g", HasExactly("h", HasExactly("i", HasExactly("j", HasExactly("k", HasExactly("l", ))))))))))),
                        "m",
                        HasExactly("n", HasExactly("o", HasExactly("p", HasExactly("q", HasExactly("r", ))))),
                        "s")))
}

func TestMultipleFirstLevelExprsPossible(t *testing.T) {
    AssertThat(t, TokenizeString("(a) (b) (c)"), HasExactly(HasExactly("a"), HasExactly("b"), HasExactly("c")))
}

func TestHandlesNumbersToo(t *testing.T) {
    AssertThat(t, TokenizeString("1"), HasExactly("1"))
}

func TestHandlesStringLiterals(t *testing.T) {
    AssertThat(t, TokenizeString("\"abc\""), HasExactly("\"abc\""))
}
