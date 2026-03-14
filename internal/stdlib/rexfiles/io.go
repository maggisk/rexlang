package rexfiles

import (
	"fmt"
	"os"

	"github.com/maggisk/rexlang/internal/eval"
)

var IOFFI = map[string]any{
	"readLine":   IO_readLine,
	"readFile":   IO_readFile,
	"writeFile":  IO_writeFile,
	"appendFile": IO_appendFile,
	"fileExists": IO_fileExists,
	"listDir":    IO_listDir,
	"print": eval.MakeBuiltin("print", func(v eval.Value) (eval.Value, error) {
		fmt.Print(eval.Display(v))
		return v, nil
	}),
	"println": eval.MakeBuiltin("println", func(v eval.Value) (eval.Value, error) {
		fmt.Println(eval.Display(v))
		return v, nil
	}),
}

func IO_readLine(prompt string) string {
	fmt.Print(prompt)
	var line string
	fmt.Scanln(&line)
	return line
}

func IO_readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func IO_writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func IO_appendFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func IO_fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func IO_listDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}
