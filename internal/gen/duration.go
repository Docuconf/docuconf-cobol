package gen

import "time"

func parseGoDuration(s string) (int64, error) {
	d, err := time.ParseDuration(s)
	return int64(d), err
}
