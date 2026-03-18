package web_server

import (
	"math"
	"math/big"
	"time"
)

func sanitizeFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func buildMarkerBucketBounds(start, end time.Time, bucketSec int) (int64, int64) {
	if bucketSec <= 0 {
		bucketSec = 300
	}
	if end.Before(start) {
		start, end = end, start
	}
	bucketSize := int64(bucketSec)
	startBucket := (start.Unix() / bucketSize) * bucketSize
	endBucket := (end.Unix() / bucketSize) * bucketSize
	return startBucket, endBucket
}

func amountToFloat(amount string, decimals int) float64 {
	if amount == "" {
		return 0
	}
	if decimals < 0 {
		decimals = 0
	}

	i, ok := new(big.Int).SetString(amount, 10)
	if !ok || i.Sign() == 0 {
		return 0
	}

	f := new(big.Float).SetInt(i)
	div := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	f.Quo(f, div)
	v, _ := f.Float64()
	return v
}
