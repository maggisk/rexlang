//go:build ignore

package main

import "os"

func Stdlib_Env_getEnv(name string) *string {
	val, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	return &val
}

func Stdlib_Env_getEnvOr(name, def string) string {
	val, ok := os.LookupEnv(name)
	if !ok {
		return def
	}
	return val
}
