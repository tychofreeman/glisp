package glisp

import (
    "testing"
    . "github.com/tychofreeman/go-matchers"
)

func TestQuoteSpitsOutRemainderOfExpression(t *testing.T) {
    AssertThat(t, Process("(quote (\"a\" \"b\" \"c\"))"), HasExactly("a", "b", "c"))
}

func TestQuotePreventsEvaluationOfParams(t *testing.T) {
    AssertThat(t, Process("(quote (plus 1 2))"), HasExactly(Symbol{"plus"}, int64(1), int64(2)))
}

func TestCarGrabsFirstItem(t *testing.T) {
    AssertThat(t, Process("(car (quote (\"a\" \"b\")))"), Equals("a"))
}

func TestCdrGrabsTail(t *testing.T) {
    AssertThat(t, Process("(cdr (quote (\"a\" \"b\" \"c\" (\"d\")))"), HasExactly("b", "c", HasExactly("d")))
}

func TestAtomIsTrueForSymbols(t *testing.T) {
    AssertThat(t, Process("(atom \"a\")"), IsTrue)
}

func TestAtomIsFalseForComplexExpres(t *testing.T) {
    AssertThat(t, Process("(atom (quote ())"), IsFalse)
}

func TestIntegerLiteralsAreImplemented(t *testing.T) {
    AssertThat(t, Process("(car (quote (1)))"), Equals(int64(1)))
}

func TestCorrectlyHandlesNestedCalls(t *testing.T) {
    AssertThat(t, Process("(car (cdr (quote (\"a\" \"b\" \"c\"))))"), Equals("b"))
}

func TestConsCreatesLists(t *testing.T) {
    AssertThat(t, Process("(cons \"a\" (quote (\"b\")))"), HasExactly("a", "b"))
}

func TestOnePlusOneEqualsTwo(t *testing.T) {
    AssertThat(t, Process("(plus 1 1)"), Equals(int64(2)))
}

func TestConditional(t *testing.T) {
    AssertThat(t, Process("(if (atom (quote ())) 1 2)"), Equals(int64(2)))
}

func TestOneEqualsOne(t *testing.T) {
    AssertThat(t, Process("(eq 1 1)"), IsTrue)
}

func TestOneNotEqualTwo(t *testing.T) {
    AssertThat(t, Process("(eq 1 2)"), IsFalse)
}

func TestSupportsLambdas(t *testing.T) {
    AssertThat(t, Process("((lambda () 6))"), Equals(int64(6)))
}

func TestSupportsExpressionsInLambdas(t *testing.T) {
    AssertThat(t, Process("((lambda () (quote (1 2 3))))"), HasExactly(Equals(int64(1)), Equals(int64(2)), Equals(int64(3))))
}

func TestSupportsLambdaParameters(t *testing.T) {
    AssertThat(t, Process("((lambda (a) a) 1)"), Equals(int64(1)))
}

func TestLambdasAreClosures(t *testing.T) {
    AssertThat(t, Process("((lambda (a) ((lambda () a))) 1)"), Equals(int64(1)))
}

func NOT_YET_DO_IT_WITH_MACROS_TestSupportsLetBindings(t *testing.T) {
    AssertThat(t, Process("(let (a 1) a)"), Equals(int64(1)))
}
