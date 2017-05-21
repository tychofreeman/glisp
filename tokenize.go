package glisp

import (
    "unicode"
    "bytes"
    "errors"
)

func TokenizeString(input string) []interface{} {
    b := bytes.NewBufferString(input)
    next := func() (byte, error) {
        if (b.Len() > 0) {
            return b.Next(1)[0], nil;
        }
        return 0, errors.New("No More Content")
    }
    return Tokenize(next)
}

func Tokenize(next func() (byte, error)) []interface{} {
    x, err := next();
    if err != nil {
        return nil
    }
    r := []interface{}{}

    acc := ""
    for ; err == nil; x, err = next() {
        c := rune(x)
        
        if unicode.IsLetter(c) || unicode.IsNumber(c) || c == '"' {
            acc += string(c)
        } else if c == '(' {
            var nested []interface{} = Tokenize(next)
            r = append(r, nested)
        } else if c == ')' {
            break
        } else {
            if acc != "" {
                r = append(r, acc)
            }
            acc = ""
        }

    }
    if acc != "" {
        r = append(r, acc)
    }
    return r
}
