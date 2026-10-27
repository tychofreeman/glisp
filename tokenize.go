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

// Tokenize reads every form in bs. It panics on unbalanced parens or an
// unterminated string.
func Tokenize(bs *bytes.Buffer) List {
	return tokenizeList(bs, false)
}

func tokenizeList(bs *bytes.Buffer, nested bool) List {
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
			for bs.Len() > 0 {
				if next, _, _ := bs.ReadRune(); next == '\n' {
					break
				}
			}
			c = '\n'
		}
		if inQuote && c != '"' {
			acc += string(c)
		} else if !unicode.IsSpace(c) && c != '(' && c != ')' && c != '"' {
			acc += string(c)
		} else if c == '(' {
			r = append(r, tokenizeList(bs, true))
		} else if c == ')' {
			if !nested {
				panic("unexpected )")
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
		panic(fmt.Sprintf("unterminated string: %s", acc))
	}
	if nested && !closed {
		panic("missing )")
	}
	if acc != "" {
		r = append(r, token(acc))
	}
	return r
}
