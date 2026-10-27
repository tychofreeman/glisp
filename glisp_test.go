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

func TestMutualRecursion(t *testing.T) {
	AssertThat(t, Process("(def x1 (lambda (x) (if (eq x 10) x (x2 (plus 1 x))))) (def x2 (lambda (x) (x1 x))) (x1 5)"), Equals(int64(10)))
	evenOdd := "(def is_even (lambda (n) (if (eq n 0) true (is_odd (- n 1))))) (def is_odd (lambda (n) (if (eq n 0) false (is_even (- n 1)))))"
	AssertThat(t, Process(evenOdd+"(is_even 10)"), IsTrue)
	AssertThat(t, Process(evenOdd+"(is_odd 7)"), IsTrue)
	AssertThat(t, Process(evenOdd+"(is_even 7)"), IsFalse)
}

// The malformed (def x2 (p x) (lambda ...)) that the old disabled mutual
// recursion test used: def evaluated (p x) as the value and ignored the rest.
func TestDefWithTheWrongNumberOfArgumentsFails(t *testing.T) {
	assertPanicsWith(t, "(def x2 (p x) (lambda (x) (x1 x)))", "def: expects a name and a value, got 3 arguments")
	assertPanicsWith(t, "(def x)", "def: expects a name and a value, got 1 argument")
	assertPanicsWith(t, "(defmacro m (lambda (xs) 1) 2)", "defmacro: expects a name and a value, got 3 arguments")
}

func TestDefnDefinesFunctions(t *testing.T) {
	AssertThat(t, Process("(defn x1 (x) (plus x 1)) (x1 4)"), Equals(int64(5)))
	AssertThat(t, Process("(defn add (a b) (plus a b)) (add 2 3)"), Equals(int64(5)))
	AssertThat(t, Process("(defn fact (n) (if (eq n 0) 1 (mult n (fact (- n 1))))) (fact 5)"), Equals(int64(120)))
}

func TestDefnBodiesCanHaveSeveralForms(t *testing.T) {
	AssertThat(t, Process("(defn f (x) (plus x 100) (plus x 1)) (f 4)"), Equals(int64(5)))
}

func TestMacrosCanExpandToLambdas(t *testing.T) {
	AssertThat(t, Process("(defmacro fn (lambda (xs) (cons (quote lambda) xs))) ((fn (x) (plus x 1)) 4)"), Equals(int64(5)))
}

func TestLetBindsNamesForItsBody(t *testing.T) {
	AssertThat(t, Process("(let (a 1) a)"), Equals(int64(1)))
	AssertThat(t, Process("(let (a 1 b 2) (plus a b))"), Equals(int64(3)))
	AssertThat(t, Process("(let () 5)"), Equals(int64(5)))
	AssertThat(t, Process("(let (a 1) (plus a 100) a)"), Equals(int64(1)))
}

func TestLetBindingsCanUseEarlierOnes(t *testing.T) {
	AssertThat(t, Process("(let (a 1 b (plus a 1)) b)"), Equals(int64(2)))
}

func TestLetShadowsOnlyInsideItsBody(t *testing.T) {
	AssertThat(t, Process("(def x 10) (let (x 1) x)"), Equals(int64(1)))
	AssertThat(t, Process("(def x 10) (let (x 1) x) x"), Equals(int64(10)))
}

func TestLetInsideAFunction(t *testing.T) {
	AssertThat(t, Process("(defn f (n) (let (m (plus n 1)) (mult m m))) (f 2)"), Equals(int64(9)))
}

func TestCanSubtractByAddingNegativeNumbers(t *testing.T) {
	AssertThat(t, Process("(plus -1 1)"), Equals(int64(0)))
}

func TestEmptyListEvaluatesToNil(t *testing.T) {
	AssertThat(t, Process("()"), IsNil)
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
	assertPanicsWith(t, "(plus 1 \"a\")", "plus: expected a number, got string \"a\"")
}

func TestPlusAddsQuotedNumbers(t *testing.T) {
	AssertThat(t, Process("(plus (car (quote (1 2))) 1)"), Equals(int64(2)))
}

func TestCarAndCdrRejectNonLists(t *testing.T) {
	assertPanicsWith(t, "(car 5)", "car: expected a list, got number 5")
	assertPanicsWith(t, "(cdr \"a\")", "cdr: expected a list, got string \"a\"")
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
	assertPanicsWith(t, "(defmacro m 5) (m)", "defmacro: expected a function, got number 5")
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
		"(plus 1 \"a\")":   "plus: expected a number, got string \"a\"",
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

func TestQuotedNumbersEqualComputedOnes(t *testing.T) {
	AssertThat(t, Process("(eq (car (quote (1))) 1)"), IsTrue)
	AssertThat(t, Process("(eq (car (quote (\"a\"))) \"a\")"), IsTrue)
}

func TestQuotedListsHoldGoValues(t *testing.T) {
	got := Process("(quote (1 \"a\" b))").(List)
	if got[0] != int64(1) || got[1] != "a" || got[2] != symbol("b") {
		t.Errorf("got %#v, want int64 1, string a, symbol b", got)
	}
}

func TestMinusSubtracts(t *testing.T) {
	AssertThat(t, Process("(- 10 3)"), Equals(int64(7)))
	AssertThat(t, Process("(- 10 3 2)"), Equals(int64(5)))
	AssertThat(t, Process("(- 5)"), Equals(int64(-5)))
	assertPanicsWith(t, "(- 1 \"a\")", "-: expected a number, got string \"a\"")
}

func TestLessAndGreaterCompare(t *testing.T) {
	AssertThat(t, Process("(< 1 2)"), IsTrue)
	AssertThat(t, Process("(< 2 1)"), IsFalse)
	AssertThat(t, Process("(< 2 2)"), IsFalse)
	AssertThat(t, Process("(> 2 1)"), IsTrue)
	AssertThat(t, Process("(> 1 2)"), IsFalse)
	assertPanicsWith(t, "(< 1)", "<: expects 2 arguments, got 1")
	assertPanicsWith(t, "(> 1 \"a\")", ">: expected a number, got string \"a\"")
}

func TestMultiplyDivideAndModulo(t *testing.T) {
	AssertThat(t, Process("(* 2 3 7)"), Equals(int64(42)))
	AssertThat(t, Process("(*)"), Equals(int64(1)))
	AssertThat(t, Process("(/ 17 5)"), Equals(int64(3)))
	AssertThat(t, Process("(/ -17 5)"), Equals(int64(-3)))
	AssertThat(t, Process("(mod 17 5)"), Equals(int64(2)))
	AssertThat(t, Process("(mod -17 5)"), Equals(int64(-2)))
	AssertThat(t, Process("(mult 6 7)"), Equals(int64(42)))
	assertPanicsWith(t, "(* 2 \"a\")", "*: expected a number, got string \"a\"")
	assertPanicsWith(t, "(/ 1 0)", "/: division by zero")
	assertPanicsWith(t, "(mod 1 0)", "mod: division by zero")
	assertPanicsWith(t, "(/ 1)", "/: expects 2 arguments, got 1")
}

func TestLessOrEqualAndGreaterOrEqual(t *testing.T) {
	AssertThat(t, Process("(<= 1 2)"), IsTrue)
	AssertThat(t, Process("(<= 2 2)"), IsTrue)
	AssertThat(t, Process("(<= 3 2)"), IsFalse)
	AssertThat(t, Process("(>= 2 2)"), IsTrue)
	AssertThat(t, Process("(>= 1 2)"), IsFalse)
	assertPanicsWith(t, "(>= 1 2 3)", ">=: expects 2 arguments, got 3")
}

func TestTrueFalseAndNilAreDefined(t *testing.T) {
	AssertThat(t, Process("true"), IsTrue)
	AssertThat(t, Process("false"), IsFalse)
	AssertThat(t, Process("nil"), IsNil)
	AssertThat(t, Process("(if true 1 2)"), Equals(int64(1)))
	AssertThat(t, Process("(eq (car (quote ())) nil)"), IsTrue)
}

func TestConsOntoNilMakesAOneElementList(t *testing.T) {
	AssertThat(t, Process("(cons 1 nil)"), HasExactly(num(1)))
	AssertThat(t, Process("(cons 1 (cons 2 nil))"), HasExactly(num(1), num(2)))
}

func TestConsWithTheWrongNumberOfArgumentsFails(t *testing.T) {
	assertPanicsWith(t, "(cons)", "cons: expects 1 or 2 arguments, got 0")
	assertPanicsWith(t, "(cons 1 2 3)", "cons: expects 1 or 2 arguments, got 3")
}

// unless expands (unless c a b) into (if c b a).
const unlessMacro = `(defmacro unless (lambda (args)
	(cons (quote if)
		(cons (car args)
			(cons (car (cdr (cdr args)))
				(cons (car (cdr args)) nil))))))`

func TestMacroExpansionsAreEvaluated(t *testing.T) {
	AssertThat(t, Process(unlessMacro+"(unless (eq 1 2) 10 20)"), Equals(int64(10)))
	AssertThat(t, Process(unlessMacro+"(unless (eq 1 1) 10 20)"), Equals(int64(20)))
}

func TestMacroExpansionsAreEvaluatedInTheCallersScope(t *testing.T) {
	AssertThat(t, Process(unlessMacro+"(def f (lambda (x) (unless (eq x 0) x 99))) (f 5)"), Equals(int64(5)))
}

func TestBuiltinsReportTheWrongNumberOfArguments(t *testing.T) {
	assertPanicsWith(t, "(if true 1)", "IF requires 3 parts")
	assertPanicsWith(t, "(eq 1)", "EQ requires exactly 2 parameters")
	assertPanicsWith(t, "(quote)", "QUOTE takes exactly 1 argument")
}

func TestCallingSomethingThatIsNotAFunctionFails(t *testing.T) {
	assertPanicsWith(t, "(1 2)", "Cannot call number 1")
	assertPanicsWith(t, "(\"f\" 2)", "Cannot call string \"f\"")
}

func TestDefNeedsASymbolToDefine(t *testing.T) {
	assertPanicsWith(t, "(def 1 2)", "def: expected a symbol to define, got number 1")
	assertPanicsWith(t, "(def)", "def: expected a symbol to define, got nil")
	assertPanicsWith(t, "(defmacro 1 (lambda (xs) 1))", "defmacro: expected a symbol to define, got number 1")
}

func TestLambdaNeedsAParameterList(t *testing.T) {
	assertPanicsWith(t, "((lambda x x) 1)", "Cannot build param bindings")
}

func TestFunctionsCanBeStoredInLists(t *testing.T) {
	AssertThat(t, Process("((car (cons plus nil)) 1 2)"), Equals(int64(3)))
}

func TestFunctionsCanBeApplied(t *testing.T) {
	AssertThat(t, Process("(def twice (lambda (f x) (f (f x)))) (twice (lambda (n) (- n 1)) 10)"), Equals(int64(8)))
	AssertThat(t, Process("(apply (lambda (a b) (- a b)) 10 3)"), Equals(int64(7)))
}

func TestRunReportsArityErrors(t *testing.T) {
	if _, err := Run("((lambda (a) a))"); err == nil || !strings.Contains(err.Error(), "expects 1 argument, got 0") {
		t.Errorf("Run of a call with too few arguments: got %v", err)
	}
}

func TestMacrosAreNotValues(t *testing.T) {
	assertPanicsWith(t, "(defmacro m (lambda (xs) 1)) (def f m)", "m is a macro, not a value")
}

// A macro applies to the forms expanded after its definition: f's body was
// expanded before later was a macro, so (later 1) is a call, and later is not
// a function.
func TestMacrosMustBeDefinedBeforeTheyAreUsed(t *testing.T) {
	assertPanicsWith(t, "(defn f () (later 1)) (defmacro later (lambda (xs) 2)) (f)", "later is a macro, not a value")
}
