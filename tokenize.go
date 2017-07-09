package glisp

import (
    "unicode"
    "bytes"
    "io/ioutil"
)

func TokenizeString(input string) List {
    b := bytes.NewBufferString(input)
    return Tokenize(b)
}

func TokenizeFile(fname string) List {
    fileBytes,err := ioutil.ReadFile(fname)
    if err != nil {
        return nil
    }
    b := bytes.NewBuffer(fileBytes)
    return Tokenize(b)
}

func Tokenize(bs *bytes.Buffer) List {
    if bs.Len() == 0 {
        return nil
    }
    r := List{}

    inQuote := false;

    acc := ""
    for ; bs.Len() > 0 ; {
        c := rune(bs.Next(1)[0])
        
        if unicode.IsLetter(c) || unicode.IsNumber(c) || c == '-' || c == '_' || c == '?' {
            acc += string(c)
        } else if c == '(' {
            nested := Tokenize(bs)
            r = append(r, nested)
        } else if c == ')' {
            break
        } else if c== '"' {
            inQuote = !inQuote
            acc += string(c)
        } else if inQuote {
            acc += string(c)
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
