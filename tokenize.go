package glisp

import (
    "unicode"
    "bytes"
    "io/ioutil"
)

func TokenizeString(input string) []interface{} {
    b := bytes.NewBufferString(input)
    return Tokenize(b)
}

func TokenizeFile(fname string) []interface{} {
    fileBytes,err := ioutil.ReadFile(fname)
    if err == nil {
        return nil
    }
    b := bytes.NewBuffer(fileBytes)
    return Tokenize(b)
}

func Tokenize(bs *bytes.Buffer) []interface{} {
    if bs.Len() == 0 {
        return nil
    }
    r := []interface{}{}

    acc := ""
    for ; bs.Len() > 0 ; {
        c := rune(bs.Next(1)[0])
        
        if unicode.IsLetter(c) || unicode.IsNumber(c) || c == '"' {
            acc += string(c)
        } else if c == '(' {
            var nested []interface{} = Tokenize(bs)
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
