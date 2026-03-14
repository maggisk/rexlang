package rexfiles

import "time"

var DateTimeFFI = map[string]any{
	"dateTimeNow":       DateTime_dateTimeNow,
	"dateTimeUtcOffset": DateTime_dateTimeUtcOffset,
}

func DateTime_dateTimeNow() int {
	return int(time.Now().UnixMilli())
}

func DateTime_dateTimeUtcOffset() int {
	_, offset := time.Now().Zone()
	return offset / 60
}
