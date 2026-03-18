//go:build ignore

package main

import "time"

func Stdlib_DateTime_dateTimeNow(_ any) int64 {
	return int64(time.Now().UnixMilli())
}

func Stdlib_DateTime_dateTimeUtcOffset(_ any) int64 {
	_, offset := time.Now().Zone()
	return int64(offset / 60)
}
