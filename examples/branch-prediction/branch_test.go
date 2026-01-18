package branchprediction

import (
	"math/rand"
	"testing"
)

type result struct {
	pos int
	neg int
}

func (r *result) reset() {
	r.pos = 0
	r.neg = 0
}

func (r *result) evalBranch(v int64) {
	if v < 0 {
		r.neg += 1
	} else {
		r.pos += 1
	}
}

func (r *result) evalBranchless(v int64) {
	// Use arithmetic to avoid branch: (v >> 63) gives -1 for negative, 0 for non-negative
	isNeg := int((v >> 63) & 1)
	r.neg += isNeg
	r.pos += 1 - isNeg
}

func TestEvalBranchless(t *testing.T) {
	tests := []struct {
		name    string
		values  []int64
		wantPos int
		wantNeg int
	}{
		{
			name:    "all negative",
			values:  []int64{-1, -2, -100, -999},
			wantPos: 0,
			wantNeg: 4,
		},
		{
			name:    "all positive",
			values:  []int64{1, 2, 100, 999},
			wantPos: 4,
			wantNeg: 0,
		},
		{
			name:    "includes zero",
			values:  []int64{-5, 0, 5},
			wantPos: 2,
			wantNeg: 1,
		},
		{
			name:    "mixed",
			values:  []int64{-1, 1, -2, 2, -3, 3},
			wantPos: 3,
			wantNeg: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r result
			for _, v := range tt.values {
				r.evalBranchless(v)
			}
			if r.pos != tt.wantPos {
				t.Errorf("pos = %d, want %d", r.pos, tt.wantPos)
			}
			if r.neg != tt.wantNeg {
				t.Errorf("neg = %d, want %d", r.neg, tt.wantNeg)
			}
		})
	}
}

const defaultSize = 100_000

func newPredictable(size int) []int64 {
	data := make([]int64, size)
	for i := range data {
		if i < size/2 {
			data[i] = -int64(i)
		} else {
			data[i] = int64(i)
		}
	}
	return data
}

func newRandom(size int) []int64 {
	rng := rand.New(rand.NewSource(42))
	data := make([]int64, size)
	for i := range data {
		data[i] = rng.Int63() - (1 << 62)
	}
	return data
}

// BenchmarkBranchWithPredictable tests branch prediction with sorted (predictable) data
func BenchmarkBranchWithPredictable(b *testing.B) {
	data := newPredictable(defaultSize)
	var r result
	for b.Loop() {
		r.reset()
		for _, v := range data {
			r.evalBranch(v)
		}
	}
	_ = r
}

// BenchmarkBranchWithRandom tests branch prediction with random (unpredictable) data
func BenchmarkBranchWithRandom(b *testing.B) {
	data := newRandom(defaultSize)
	var r result
	for b.Loop() {
		r.reset()
		for _, v := range data {
			r.evalBranch(v)
		}
	}
	_ = r
}

// BenchmarkBranchlessWithPredictable tests branchless version with sorted data
func BenchmarkBranchlessWithPredictable(b *testing.B) {
	data := newPredictable(defaultSize)
	var r result
	for b.Loop() {
		r.reset()
		for _, v := range data {
			r.evalBranchless(v)
		}
	}
	_ = r
}

// BenchmarkBranchlessWithRandom tests branchless version with random data
func BenchmarkBranchlessWithRandom(b *testing.B) {
	data := newRandom(defaultSize)
	var r result
	for b.Loop() {
		r.reset()
		for _, v := range data {
			r.evalBranchless(v)
		}
	}
	_ = r
}
