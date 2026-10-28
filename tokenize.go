package glisp

import (
	"bytes"
	"fmt"
	"os"
	"unicode"
)

func TokenizeString(input string) List {
	b := bytes.NewBufferString(input)
	return tokenizeList(b, false)
}

func TokenizeFile(fname string) List {
	fileBytes, err := os.ReadFile(fname)
	if err != nil {
		return nil
	}
	b := bytes.NewBuffer(fileBytes)
	return tokenizeList(b, false)
}

// Tokenize reads every form in bs. It panics on unbalanced parens, an
// unterminated string or an unknown escape in one.
func Tokenize(bs *bytes.Buffer) List {
	return tokenizeList(bs, false)
}

func tokenizeList(bs *bytes.Buffer, nested bool) List {
	return (&tokenizer{bs: bs, line: 1, lines: Lines{}}).list(nested)
}

// Lines records the line each list read from source starts on, by the
// address of its first element: a list is a slice, and its first element
// stays put while it is read, copied by readLines and expanded.
type Lines map[*interface{}]int

// tokenizer reads forms from source, counting lines.
type tokenizer struct {
	bs    *bytes.Buffer
	line  int
	lines Lines
}

// tokenizeLines reads every form in input, and the line each list starts on.
func tokenizeLines(input string) (List, Lines) {
	t := &tokenizer{bs: bytes.NewBufferString(input), line: 1, lines: Lines{}}
	return t.list(false), t.lines
}

func (t *tokenizer) list(nested bool) List {
	bs := t.bs
	if bs.Len() == 0 && !nested {
		return nil
	}
	r := List{}

	inQuote := false

	closed := false
	acc := ""
	for bs.Len() > 0 {
		c, _, _ := bs.ReadRune()

		// A comment runs from ; to the end of the line and separates tokens
		// like the newline it ends with.
		if !inQuote && c == ';' {
			c = ' '
			for bs.Len() > 0 {
				if next, _, _ := bs.ReadRune(); next == '\n' {
					c = '\n'
					break
				}
			}
		}
		if c == '\n' {
			t.line++
		}
		if inQuote && c == '\\' {
			acc += t.escape()
		} else if inQuote && c != '"' {
			acc += string(c)
		} else if !unicode.IsSpace(c) && c != '(' && c != ')' && c != '"' {
			acc += string(c)
		} else if c == '(' {
			start := t.line
			sub := t.list(true)
			if len(sub) > 0 {
				t.lines[&sub[0]] = start
			}
			r = append(r, sub)
		} else if c == ')' {
			if !nested {
				panic(fmt.Sprintf("line %d: unexpected )", t.line))
			}
			closed = true
			break
		} else if c == '"' {
			inQuote = !inQuote
			acc += string(c)
		} else {
			if acc != "" {
				r = append(r, token(acc))
			}
			acc = ""
		}

	}
	if inQuote {
		panic(fmt.Sprintf("line %d: unterminated string: %s", t.line, acc))
	}
	if nested && !closed {
		panic(fmt.Sprintf("line %d: missing )", t.line))
	}
	if acc != "" {
		r = append(r, token(acc))
	}
	return r
}

// escape reads an escape in a string, after its backslash, and returns the
// character it stands for: \" and \\ themselves, \n a newline, \t a tab.
// At the end of the input it returns nothing, and the string is
// unterminated.
func (t *tokenizer) escape() string {
	if t.bs.Len() == 0 {
		return ""
	}
	c, _, _ := t.bs.ReadRune()
	switch c {
	case '"', '\\':
		return string(c)
	case 'n':
		return "\n"
	case 't':
		return "\t"
	}
	panic(fmt.Sprintf("line %d: unknown escape \\%c in a string", t.line, c))
}
