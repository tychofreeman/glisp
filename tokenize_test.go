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
    AssertThat(t, TokenizeString("(a)"), HasExactly(HasExactly(Sym("a"))))
}

func TestAddsMultipleSymbolNamesToListInOrder(t *testing.T) {
    AssertThat(t, TokenizeString("(a b c)"), HasExactly(HasExactly(Sym("a"), Sym("b"), Sym("c"))))
}

func TestSymbolNamesCanHaveMultipleLetters(t *testing.T) {
    AssertThat(t, TokenizeString("(abc def ghi)"), HasExactly(HasExactly(Sym("abc"), Sym("def"), Sym("ghi"))))
}

func TestParensCreatesANestedList(t *testing.T) {
    AssertThat(t, TokenizeString("(a (b))"), HasExactly(HasExactly(Sym("a"), HasExactly(Sym("b")))))
}

func TestNestedListsCanComeInTheMiddleToo(t *testing.T) {
    AssertThat(t, TokenizeString("(a (b) c)"), HasExactly(HasExactly(Sym("a"), HasExactly(Sym("b")), Sym("c"))))
}

func TestDeeplyNestedListsWorkToo(t *testing.T) {
    AssertThat(t, TokenizeString("(a (b (c (d (e (f (g (h (i (j (k (l))))))))))) m (n (o (p (q (r))))) s)"), 
        HasExactly(
            HasExactly(Sym("a"),
                        HasExactly(Sym("b"), HasExactly(Sym("c"), HasExactly(Sym("d"), HasExactly(Sym("e"), HasExactly(Sym("f"), HasExactly(Sym("g"), HasExactly(Sym("h"), HasExactly(Sym("i"), HasExactly(Sym("j"), HasExactly(Sym("k"), HasExactly(Sym("l"), ))))))))))),
                        Sym("m"),
                        HasExactly(Sym("n"), HasExactly(Sym("o"), HasExactly(Sym("p"), HasExactly(Sym("q"), HasExactly(Sym("r"), ))))),
                        Sym("s"))))
}

func TestMultipleFirstLevelExprsPossible(t *testing.T) {
    AssertThat(t, TokenizeString("(a) (b) (c)"), HasExactly(HasExactly(Sym("a")), HasExactly(Sym("b")), HasExactly(Sym("c"))))
}

func TestHandlesNumbersToo(t *testing.T) {
    AssertThat(t, TokenizeString("1"), HasExactly(Sym("1")))
}

func TestHandlesStringLiterals(t *testing.T) {
    AssertThat(t, TokenizeString("\"abc\""), HasExactly(Sym("\"abc\"")))
}

func TestHandlesStringsWithSpaces(t *testing.T) {
    AssertThat(t, TokenizeString("\"abc def\" \"ghi jkl\""), HasExactly(Sym("\"abc def\""), Sym("\"ghi jkl\"")))
}

func TestHandlesNegativeNumbers(t *testing.T) {
    AssertThat(t, TokenizeString("-1"), HasExactly(Sym("-1")))
}

func TestHandlesUnderscoresInNames(t *testing.T) {
    AssertThat(t, TokenizeString("\"a_b\""), HasExactly(Sym("\"a_b\"")))
}
