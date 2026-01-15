package timeutil

import (
	"sync"
	"time"
)

const (
	DefaultLocationName  = "Asia/Shanghai"
	defaultOffsetSeconds = 8 * 60 * 60
)

var (
	locOnce sync.Once
	loc     *time.Location
)

func Init() *time.Location {
	locOnce.Do(func() {
		loaded, err := time.LoadLocation(DefaultLocationName)
		if err != nil {
			loaded = time.FixedZone("UTC+8", defaultOffsetSeconds)
		}
		loc = loaded
		time.Local = loaded
	})
	return loc
}

func Location() *time.Location {
	if loc == nil {
		return Init()
	}
	return loc
}

func LocationName() string {
	return DefaultLocationName
}

func Now() time.Time {
	return time.Now().In(Location())
}
