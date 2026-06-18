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
	offset := time.Duration(hashKey(key) % int64(window))
	ttl := base - offset
	if ttl <= 0 {
		return time.Nanosecond
	}
	return ttl
}

func hashKey(key string) int64 {
	high := hashKey32("high:" + key)
	low := hashKey32("low:" + key)
	return (int64(high&0x3fffffff) << 32) | int64(low)
}

func hashKey32(key string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return hash.Sum32()
}
