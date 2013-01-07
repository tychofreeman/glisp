package glisp

import (
    "unicode"
    "bytes"
)

func TokenizeString(input string) ([]interface{}) {
    return Tokenize(bytes.NewBufferString(input))
}

func Tokenize(buf *bytes.Buffer) ([]interface{}) {
    if buf.Len() == 0 {
        return nil
    }
    r := []interface{}{}

    acc := ""
    for ; buf.Len() > 0; {
        c := rune(buf.Next(1)[0])
        
        if unicode.IsLetter(c) {
            acc += string(c)
        } else if c == '(' {
            var nested []interface{} = Tokenize(buf)
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
