//go:build ignore

package main

import (
	"fmt"
	"os"
)

func Stdlib_IO_print(v any) any {
	fmt.Print(rex_display(v))
	return v
}

func Stdlib_IO_println(v any) any {
	fmt.Println(rex_display(v))
	return v
}

func Stdlib_IO_readLine(prompt string) string {
	fmt.Print(prompt)
	var line string
	fmt.Scanln(&line)
	return line
}

func Stdlib_IO_readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func Stdlib_IO_writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func Stdlib_IO_appendFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func Stdlib_IO_fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func Stdlib_IO_listDir(path string) (*RexList, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var list *RexList
	for i := len(entries) - 1; i >= 0; i-- {
		list = &RexList{Head: entries[i].Name(), Tail: list}
	}
	return list, nil
}
