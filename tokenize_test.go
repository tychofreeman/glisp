package glisp

import (
	"testing"

	// Use github version as soon as changes are uploaded.
	. "github.com/tychofreeman/go-matchers"
	//. "matchers"
)

func TestEmptyStringYieldsNilList(t *testing.T) {
	AssertThat(t, TokenizeString(""), IsEmpty)
}

func TestBareParensYieldsEmptyList(t *testing.T) {
	AssertThat(t, TokenizeString("()"), HasExactly(HasExactly()))
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
				HasExactly(symbol("b"), HasExactly(symbol("c"), HasExactly(symbol("d"), HasExactly(symbol("e"), HasExactly(symbol("f"), HasExactly(symbol("g"), HasExactly(symbol("h"), HasExactly(symbol("i"), HasExactly(symbol("j"), HasExactly(symbol("k"), HasExactly(symbol("l")))))))))))),
				symbol("m"),
				HasExactly(symbol("n"), HasExactly(symbol("o"), HasExactly(symbol("p"), HasExactly(symbol("q"), HasExactly(symbol("r")))))),
				symbol("s"))))
}

func TestMultipleFirstLevelExprsPossible(t *testing.T) {
	AssertThat(t, TokenizeString("(a) (b) (c)"), HasExactly(HasExactly(symbol("a")), HasExactly(symbol("b")), HasExactly(symbol("c"))))
}

// ` , and ,@ before a form read as (quasiquote form), (unquote form) and
// (unquote-splicing form).
func TestQuasiquoteMarksReadAsLists(t *testing.T) {
	q := func(name string, x interface{}) List { return List{symbol(name), x} }
	AssertThat(t, TokenizeString("`(a ,b ,@(c) ,`d)"), Equals(List{q("quasiquote",
		List{symbol("a"), q("unquote", symbol("b")), q("unquote-splicing", List{symbol("c")}),
			q("unquote", q("quasiquote", symbol("d")))})}))
	AssertThat(t, TokenizeString("a,b"), Equals(List{symbol("a,b")}))
	assertPanicsWith(t, "(a `)", "line 1: ` with nothing after it")
}

func TestHandlesNumbersToo(t *testing.T) {
	AssertThat(t, TokenizeString("1"), HasExactly(token("1")))
}

func TestHandlesStringLiterals(t *testing.T) {
	AssertThat(t, TokenizeString("\"abc\""), HasExactly(token("\"abc\"")))
}

func TestHandlesStringsWithSpaces(t *testing.T) {
	AssertThat(t, TokenizeString("\"abc def\" \"ghi jkl\""), HasExactly(token("\"abc def\""), token("\"ghi jkl\"")))
}

func TestHandlesNegativeNumbers(t *testing.T) {
	AssertThat(t, TokenizeString("-1"), HasExactly(token("-1")))
}

func TestHandlesUnderscoresInNames(t *testing.T) {
	AssertThat(t, TokenizeString("\"a_b\""), HasExactly(token("\"a_b\"")))
}

func TestParensInsideStringsArePartOfTheString(t *testing.T) {
	AssertThat(t, TokenizeString("(quote \"a(b)c\")"), HasExactly(HasExactly(symbol("quote"), token("\"a(b)c\""))))
}

func TestOperatorCharactersAreSymbols(t *testing.T) {
	AssertThat(t, TokenizeString("(+ 1 2)"), HasExactly(HasExactly(symbol("+"), token("1"), token("2"))))
	AssertThat(t, TokenizeString("(<= a.b *x*)"), HasExactly(HasExactly(symbol("<="), symbol("a.b"), symbol("*x*"))))
}

func TestUnterminatedStringsFail(t *testing.T) {
	assertPanicsWith(t, "(quote \")", "unterminated string")
	assertPanicsWith(t, "\"abc", "unterminated string: \"abc")
}

func TestNonASCIITextIsKept(t *testing.T) {
	AssertThat(t, TokenizeString("(λ \"héllo\")"), HasExactly(HasExactly(symbol("λ"), token("\"héllo\""))))
}

func TestUnbalancedParensFail(t *testing.T) {
	assertPanicsWith(t, "(plus 1 2)) (plus 10 10)", "unexpected )")
	assertPanicsWith(t, "(plus 1 (plus 2 3)", "missing )")
	assertPanicsWith(t, "(", "missing )")
}

func TestCommentsRunToTheEndOfTheLine(t *testing.T) {
	AssertThat(t, Process("; leading\n(plus 1 2) ; trailing\n; last line"), Equals(int64(3)))
	AssertThat(t, Process("(plus 1 2;no space before it\n)"), Equals(int64(3)))
}

func TestSemicolonsInStringsAreNotComments(t *testing.T) {
	AssertThat(t, Process("(quote \"a;b\")"), Equals("a;b"))
}
