package semcache

import (
	"math"
	"sync"
	"time"
)

// nsIndex is one namespace's flat cosine index. Vectors are stored in a single
// contiguous row-major matrix for cache locality; the mutex closes the
// read-modify-write race the old token index had.
type nsIndex struct {
	mu        sync.Mutex
	dim       int
	vectors   []float32 // row-major, len = count*dim, each row L2-normalized
	storeKeys []string
	expires   []time.Time
	seqs      []uint64
	capacity  int
	seqCtr    uint64
}

func newNSIndex(capacity int) *nsIndex {
	if capacity <= 0 {
		capacity = 1000
	}
	return &nsIndex{capacity: capacity}
}

func (idx *nsIndex) size() int {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return len(idx.storeKeys)
}

// add inserts (or refreshes) a normalized vector + storeKey, evicting the
// least-recently-used entry when at capacity.
func (idx *nsIndex) add(vector []float32, storeKey string, expires time.Time) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.seqCtr++
	if idx.dim == 0 {
		idx.dim = len(vector)
	}
	if len(vector) != idx.dim {
		// Embedding dimension changed (fingerprint/model change mid-flight) —
		// reset the index rather than mix dimensions.
		idx.vectors = idx.vectors[:0]
		idx.storeKeys = idx.storeKeys[:0]
		idx.expires = idx.expires[:0]
		idx.seqs = idx.seqs[:0]
		idx.dim = len(vector)
	}
	d := idx.dim
	for i, k := range idx.storeKeys {
		if k == storeKey {
			copy(idx.vectors[i*d:(i+1)*d], vector)
			idx.expires[i] = expires
			idx.seqs[i] = idx.seqCtr
			return
		}
	}
	if idx.capacity > 0 && len(idx.storeKeys) >= idx.capacity {
		minIdx := 0
		for i := range idx.seqs {
			if idx.seqs[i] < idx.seqs[minIdx] {
				minIdx = i
			}
		}
		idx.removeAt(minIdx)
	}
	idx.vectors = append(idx.vectors, vector...)
	idx.storeKeys = append(idx.storeKeys, storeKey)
	idx.expires = append(idx.expires, expires)
	idx.seqs = append(idx.seqs, idx.seqCtr)
}

// removeAt swap-removes row i (order-independent; recency lives in seqs).
func (idx *nsIndex) removeAt(i int) {
	last := len(idx.storeKeys) - 1
	d := idx.dim
	copy(idx.vectors[i*d:(i+1)*d], idx.vectors[last*d:(last+1)*d])
	idx.vectors = idx.vectors[:last*d]
	idx.storeKeys[i] = idx.storeKeys[last]
	idx.storeKeys = idx.storeKeys[:last]
	idx.expires[i] = idx.expires[last]
	idx.expires = idx.expires[:last]
	idx.seqs[i] = idx.seqs[last]
	idx.seqs = idx.seqs[:last]
}

// best returns the storeKey with the highest cosine similarity ≥ threshold, or
// "" if none. Expired entries are skipped; a hit is marked recently used.
func (idx *nsIndex) best(query []float32, threshold float64, now time.Time) (string, float64) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.dim == 0 || len(query) != idx.dim {
		return "", 0
	}
	bestIdx, bestScore := -1, float32(threshold)
	d := idx.dim
	for i := 0; i < len(idx.storeKeys); i++ {
		if e := idx.expires[i]; !e.IsZero() && now.After(e) {
			continue
		}
		if s := dotF32(query, idx.vectors[i*d:(i+1)*d]); s >= bestScore {
			bestScore, bestIdx = s, i
		}
	}
	if bestIdx < 0 {
		return "", 0
	}
	idx.seqCtr++
	idx.seqs[bestIdx] = idx.seqCtr
	return idx.storeKeys[bestIdx], float64(bestScore)
}

func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	inv := float32(1 / math.Sqrt(sum))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x * inv
	}
	return out
}

// dotF32 is the cosine similarity of two L2-normalized vectors, 4-way unrolled
// with float32 accumulation for a tight cache-local scan.
func dotF32(a, b []float32) float32 {
	var s0, s1, s2, s3 float32
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for ; i+4 <= n; i += 4 {
		s0 += a[i] * b[i]
		s1 += a[i+1] * b[i+1]
		s2 += a[i+2] * b[i+2]
		s3 += a[i+3] * b[i+3]
	}
	for ; i < n; i++ {
		s0 += a[i] * b[i]
	}
	return s0 + s1 + s2 + s3
}
