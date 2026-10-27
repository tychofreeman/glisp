package glisp

import (
	"fmt"
	"strings"
	"sync"

	. "github.com/tychofreeman/go-matchers"
	"testing"
)

func TestQuoteSpitsOutRemainderOfExpression(t *testing.T) {
	AssertThat(t, Process("(quote (\"a\" \"b\" \"c\"))"), HasExactly(str("a"), str("b"), str("c")))
}

func TestQuotePreventsEvaluationOfParams(t *testing.T) {
	AssertThat(t, Process("(quote (plus 1 2))"), HasExactly(symbol("plus"), num(1), num(2)))
}

func TestCarGrabsFirstItem(t *testing.T) {
	AssertThat(t, Process("(car (quote (\"a\" \"b\")))"), Equals(str("a")))
}

func TestCdrGrabsTail(t *testing.T) {
	AssertThat(t, Process("(cdr (quote (\"a\" \"b\" \"c\" (\"d\"))))"), HasExactly(str("b"), str("c"), HasExactly(str("d"))))
}

func TestAtomIsTrueForSymbols(t *testing.T) {
	AssertThat(t, Process("(atom \"a\")"), IsTrue)
}

func TestAtomIsFalseForComplexExpres(t *testing.T) {
	AssertThat(t, Process("(atom (quote ()))"), IsFalse)
}

func TestIntegerLiteralsAreImplemented(t *testing.T) {
	AssertThat(t, Process("(car (quote (1)))"), Equals(num(1)))
}

func TestCorrectlyHandlesNestedCalls(t *testing.T) {
	AssertThat(t, Process("(car (cdr (quote (\"a\" \"b\" \"c\"))))"), Equals(str("b")))
}

func TestConsCreatesLists(t *testing.T) {
	AssertThat(t, Process("(cons \"a\" (quote (\"b\")))"), HasExactly(str("a"), str("b")))
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
	AssertThat(t, Process("((lambda () (quote (1 2 3))))"), HasExactly(num(1), num(2), num(3)))
}

func TestSupportsLambdaParameters(t *testing.T) {
	AssertThat(t, Process("((lambda (a) a) 1)"), Equals(int64(1)))
}

func TestLambdasAreClosures(t *testing.T) {
	AssertThat(t, Process("((lambda (a) ((lambda () a))) 1)"), Equals(int64(1)))
}

func TestApplyCallsFunctions(t *testing.T) {
	AssertThat(t, Process("(apply plus 1 1)"), Equals(int64(2)))
}

func TestGloballyNamesFunctions(t *testing.T) {
	AssertThat(t, Process("(def thing (lambda (a) (plus a a))) (thing 5)"), Equals(int64(10)))
}

func TestGloballyDefinedMacros(t *testing.T) {
	AssertThat(t, Process("(defmacro five (lambda (xs) 5)) (five one two three)"), Equals(int64(5)))
}

func TestGloballyDefinedMacrosCanCallFunctions(t *testing.T) {
	AssertThat(t, Process("(def double (lambda (x) (plus x x))) (defmacro five (lambda (xs) (double 5))) (five one two three)"), Equals(int64(10)))
}

// Amusingly, macros need params. I don't feel like fixing this. :-)
func TestMacrosCanNest(t *testing.T) {
	AssertThat(t, Process("(defmacro five (lambda (xs) 5)) (defmacro ten (lambda (xs) (plus (five asdf) (five asdf)))) (ten asdf)"), Equals(int64(10)))
}

func TestCanPullOffYCombinatorTypeTrick(t *testing.T) {
	AssertThat(t, Process("(def r1 (lambda (x r) (if (eq 10 x) \"true\" (r (plus 1 x) r)))) (r1 0 r1)"), Equals("true"))
}

func THIS_FAILS_FIND_A_GOOD_WAY_TO_CATCH_THIS_ERROR_TestMutualRecursionFails(t *testing.T) {
	AssertThat(t, Process("(def x1 (lambda (x) (p x) (if (eq x 10) x (x2 (plus 1 x))))) (def x2 (p x) (lambda (x) (x1 x))) (x1 5)"), Equals(int64(0)))
}

func COMPILE_IN_MULTIPLE_PASSES_TO_GET_THIS_TO_WORK_TestFunctionsCanBeDefinedWithShortcuts(t *testing.T) {
	AssertThat(t, Process("(defmacro defn (lambda (xs) (def (car (xs) (cdr xs))))) (defn x1 (x) (plus x 1)) (x1 4)"), Equals(int64(5)))
	AssertThat(t, Process("(defn x1 (x) (plus x 1)) (x1 4)"), Equals(int64(5)))
}

func NOT_YET_DO_IT_WITH_MACROS_TestSupportsLetBindings(t *testing.T) {
	AssertThat(t, Process("(let (a 1) a)"), Equals(int64(1)))
}

func TestCanSubtractByAddingNegativeNumbers(t *testing.T) {
	AssertThat(t, Process("(plus -1 1)"), Equals(int64(0)))
}

func TestEmptyListEvaluatesToNil(t *testing.T) {
	AssertThat(t, GetValue(builtinScope, List{}) == nil, IsTrue)
}

func TestStdlibIsLoaded(t *testing.T) {
	AssertThat(t, Process("(mult 3 4)"), Equals(int64(12)))
}

func TestDefStoresTheValueOfItsExpression(t *testing.T) {
	AssertThat(t, Process("(def y (plus 1 2)) (plus y 1)"), Equals(int64(4)))
}

func TestFunctionsCanBePassedAsArguments(t *testing.T) {
	AssertThat(t, Process("((lambda (f x) (f x)) (lambda (n) (plus n 1)) 41)"), Equals(int64(42)))
}

func TestFunctionsCanBeReturned(t *testing.T) {
	AssertThat(t, Process("(def make_inc (lambda () (lambda (n) (plus n 1)))) ((make_inc) 41)"), Equals(int64(42)))
}

func TestClosuresCaptureTheirDefiningScope(t *testing.T) {
	AssertThat(t, Process("(def make_adder (lambda (x) (lambda (y) (plus x y)))) (def add5 (make_adder 5)) (add5 10)"), Equals(int64(15)))
}

func TestFreeVariablesDoNotSeeTheCallersParameters(t *testing.T) {
	AssertThat(t, Process("(def outer_v 100) (def get_v (lambda () outer_v)) (def shadow (lambda (outer_v) (get_v))) (shadow 1)"), Equals(int64(100)))
}

// assertPanicsWith checks that running src panics with a message containing want.
func assertPanicsWith(t *testing.T, src string, want string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("%s: expected a panic containing %q, got none", src, want)
		} else if !strings.Contains(fmt.Sprint(r), want) {
			t.Errorf("%s: expected a panic containing %q, got %v", src, want, r)
		}
	}()
	Process(src)
}

func TestCallingALambdaWithTooFewArgumentsFails(t *testing.T) {
	assertPanicsWith(t, "((lambda (a b) a) 1)", "expects 2 arguments, got 1")
}

func TestCallingALambdaWithTooManyArgumentsFails(t *testing.T) {
	assertPanicsWith(t, "((lambda (a) a) 1 2)", "expects 1 argument, got 2")
}

func TestEqComparesListsByContents(t *testing.T) {
	AssertThat(t, Process("(eq (quote (1 (2 3))) (quote (1 (2 3))))"), IsTrue)
	AssertThat(t, Process("(eq (quote (1 2)) (quote (1 3)))"), IsFalse)
	AssertThat(t, Process("(eq (quote (1 2)) (quote (1 2 3)))"), IsFalse)
	AssertThat(t, Process("(eq (quote (1)) 1)"), IsFalse)
}

func TestEqOnFunctionsDoesNotCrash(t *testing.T) {
	AssertThat(t, Process("(eq plus car)"), IsFalse)
}

func TestPlusRejectsNonNumbers(t *testing.T) {
	assertPanicsWith(t, "(plus 1 \"a\")", "plus: expected a number, got string a")
}

func TestPlusAddsQuotedNumbers(t *testing.T) {
	AssertThat(t, Process("(plus (car (quote (1 2))) 1)"), Equals(int64(2)))
}

func TestCarAndCdrRejectNonLists(t *testing.T) {
	assertPanicsWith(t, "(car 5)", "car: expected a list, got int64 5")
	assertPanicsWith(t, "(cdr \"a\")", "cdr: expected a list, got string a")
}

func TestCarAndCdrOfNilAreNil(t *testing.T) {
	AssertThat(t, Process("(car (cdr (quote ())))"), IsNil)
	AssertThat(t, Process("(cdr (cdr (quote ())))"), IsNil)
}

// IsNil matches a nil value.
func IsNil(actual interface{}) (bool, string) {
	return actual == nil, fmt.Sprintf("expected nil, got %T %v", actual, actual)
}

func TestDefmacroOfANonFunctionFails(t *testing.T) {
	assertPanicsWith(t, "(defmacro m 5) (m)", "defmacro: expected a function, got int64 5")
}

func TestDefinitionsDoNotLeakBetweenRuns(t *testing.T) {
	Process("(def leaked_def 1)")
	assertPanicsWith(t, "leaked_def", "Cannot resolve symbol leaked_def")
}

func TestDefDoesNotChangeTheBuiltins(t *testing.T) {
	Process("(def car 5) (def builtin_leak 1)")
	if _, isFunction := builtins["car"].(Function); !isFunction {
		t.Errorf("builtins[car] was replaced: %v", builtins["car"])
	}
	if _, found := builtins["builtin_leak"]; found {
		t.Errorf("def added builtin_leak to the builtins")
	}
}

func TestRunsCanHappenConcurrently(t *testing.T) {
	var wg sync.WaitGroup
	for i := int64(0); i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			src := fmt.Sprintf("(def n %d) (def twice (lambda (x) (plus x x))) (twice n)", i)
			if got := Process(src); got != i+i {
				t.Errorf("%s: got %v, want %d", src, got, i+i)
			}
		}()
	}
	wg.Wait()
}

func TestRunReturnsTheValue(t *testing.T) {
	value, err := Run("(plus 1 2)")
	if err != nil || value != int64(3) {
		t.Errorf("Run: got %v, %v; want 3, nil", value, err)
	}
}

func TestRunReturnsErrorsInsteadOfPanicking(t *testing.T) {
	for src, want := range map[string]string{
		"(plus 1 \"a\")":   "plus: expected a number, got string a",
		"(undefined_fn 1)": "Cannot resolve symbol undefined_fn",
		"(plus 1 2":        "missing )",
	} {
		value, err := Run(src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Run(%q): got %v, %v; want an error containing %q", src, value, err, want)
		}
	}
}

func TestRunFileReportsAMissingFile(t *testing.T) {
	if _, err := RunFile("no/such/file.glisp"); err == nil {
		t.Errorf("RunFile of a missing file: got no error")
	}
}
