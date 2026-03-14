package rexfiles

import "os"

var EnvFFI = map[string]any{
	"getEnv":   Env_getEnv,
	"getEnvOr": Env_getEnvOr,
}

func Env_getEnv(name string) *string {
	val, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	return &val
}

func Env_getEnvOr(name, def string) string {
	val, ok := os.LookupEnv(name)
	if !ok {
		return def
	}
	return val
}
