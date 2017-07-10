package glisp

import (
    "testing"

    // Use github version as soon as changes are uploaded.
    . "github.com/tychofreeman/go-matchers"
    //. "matchers"
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
    AssertThat(t, TokenizeString("(a)"), HasExactly(HasExactly(symbol("a"))))
}

func TestAddsMultipleSymbolNamesToListInOrder(t *testing.T) {
    AssertThat(t, TokenizeString("(a b c)"), HasExactly(HasExactly(symbol("a"), symbol("b"), symbol("c"))))
}

func TestSymbolNamesCanHaveMultipleLetters(t *testing.T) {
    AssertThat(t, TokenizeString("(abc def ghi)"), HasExactly(HasExactly(symbol("abc"), symbol("def"), symbol("ghi"))))
}

func TestParensCreatesANestedList(t *testing.T) {
    AssertThat(t, TokenizeString("(a (b))"), HasExactly(HasExactly(symbol("a"), HasExactly(symbol("b")))))
}

func TestNestedListsCanComeInTheMiddleToo(t *testing.T) {
    AssertThat(t, TokenizeString("(a (b) c)"), HasExactly(HasExactly(symbol("a"), HasExactly(symbol("b")), symbol("c"))))
}

func TestDeeplyNestedListsWorkToo(t *testing.T) {
    AssertThat(t, TokenizeString("(a (b (c (d (e (f (g (h (i (j (k (l))))))))))) m (n (o (p (q (r))))) s)"), 
        HasExactly(
            HasExactly(symbol("a"),
                        HasExactly(symbol("b"), HasExactly(symbol("c"), HasExactly(symbol("d"), HasExactly(symbol("e"), HasExactly(symbol("f"), HasExactly(symbol("g"), HasExactly(symbol("h"), HasExactly(symbol("i"), HasExactly(symbol("j"), HasExactly(symbol("k"), HasExactly(symbol("l"), ))))))))))),
                        symbol("m"),
                        HasExactly(symbol("n"), HasExactly(symbol("o"), HasExactly(symbol("p"), HasExactly(symbol("q"), HasExactly(symbol("r"), ))))),
                        symbol("s"))))
}

func TestMultipleFirstLevelExprsPossible(t *testing.T) {
    AssertThat(t, TokenizeString("(a) (b) (c)"), HasExactly(HasExactly(symbol("a")), HasExactly(symbol("b")), HasExactly(symbol("c"))))
}

func TestHandlesNumbersToo(t *testing.T) {
    AssertThat(t, TokenizeString("1"), HasExactly(symbol("1")))
}

func TestHandlesStringLiterals(t *testing.T) {
    AssertThat(t, TokenizeString("\"abc\""), HasExactly(symbol("\"abc\"")))
}

func TestHandlesStringsWithSpaces(t *testing.T) {
    AssertThat(t, TokenizeString("\"abc def\" \"ghi jkl\""), HasExactly(symbol("\"abc def\""), symbol("\"ghi jkl\"")))
}

func TestHandlesNegativeNumbers(t *testing.T) {
    AssertThat(t, TokenizeString("-1"), HasExactly(symbol("-1")))
}

func TestHandlesUnderscoresInNames(t *testing.T) {
    AssertThat(t, TokenizeString("\"a_b\""), HasExactly(symbol("\"a_b\"")))
}
