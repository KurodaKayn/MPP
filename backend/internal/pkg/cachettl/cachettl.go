package cachettl

import (
	"hash/fnv"
	"time"
)

const defaultJitterPercent = 20

// Jitter returns a deterministic per-key TTL in [base-jitter, base].
func Jitter(base time.Duration, key string) time.Duration {
	return JitterPercent(base, key, defaultJitterPercent)
}

func JitterPercent(base time.Duration, key string, percent int) time.Duration {
	if base <= 0 || percent <= 0 {
		return base
	}
	if percent > 100 {
		percent = 100
	}
	window := base * time.Duration(percent) / 100
	if window <= 0 {
		return base
	}
	offset := time.Duration(hashKey(key) % uint64(window+1))
	ttl := base - offset
	if ttl <= 0 {
		return time.Nanosecond
	}
	return ttl
}

func hashKey(key string) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(key))
	return hash.Sum64()
}
