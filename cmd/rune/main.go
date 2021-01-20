package main

import (
	"fmt"
	"unicode/utf8"
)

func main() {
	s := "Golang编程"
	fmt.Printf("byte len of s: %d\n", len(s))                    //output: 12(按字节编码是12个字节，‘编码’每个中文字符3个字节)
	fmt.Printf("rune len of s: %d\n", utf8.RuneCountInString(s)) //output: 8

	proc("王二小", func(str string){
		for _, v := range str {  //range字符串, v是rune类型
			fmt.Printf("%c\n", v)
		}
	})
}

func proc(input string, processor func(str string)) {
	processor(input)
}