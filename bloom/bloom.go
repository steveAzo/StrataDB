package bloom

// Filter is a simple bloom filter using a bit array and double hashing.
// It can report that a key is DEFINITELY absent (MayContain=false) or
// POSSIBLY present (MayContain=true, with a small false-positive rate).
//
// We use double hashing to simulate K hash functions cheaply:
//   h_i(key) = (h1 + i*h2) % numBits
// where h1 and h2 come from a single FNV-1a pass over the key.
type Filter struct {
	bits    []byte // bit array; bit i lives at bits[i/8], shift (i%8)
	numBits uint64
}

// New creates a Filter with numBits capacity.
// More bits → fewer false positives but more memory.
// Rule of thumb: ~10 bits per expected key gives ~1% false positive rate.
func New(numBits uint64) *Filter {
	return &Filter{
		bits:    make([]byte, (numBits+7)/8), // round up to nearest byte
		numBits: numBits,
	}
}

// Add inserts key into the filter by setting K=3 bits derived from double hashing.
func (f *Filter) Add(key string) {
	h1, h2 := hashKey(key)
	for i := uint64(0); i < 3; i++ {
		bit := (h1 + i*h2) % f.numBits
		f.bits[bit/8] |= 1 << (bit % 8)
	}
}

// MayContain returns false if key is DEFINITELY not in the filter,
// or true if it MIGHT be (false positives are possible).
func (f *Filter) MayContain(key string) bool {

	h1, h2 := hashKey(key) 
	for i := uint64(0); i < 3; i++ {
		bit := (h1 + i*h2) % f.numBits 
		if f.bits[bit/8]&(1<<(bit%8)) == 0 {
			return false
		}
	}
	return true
}

// hashKey returns two independent 64-bit hashes of key using FNV-1a.
// h1 is a standard FNV-1a hash; h2 is FNV-1a with a different offset,
// giving us two uncorrelated values for double hashing.
func hashKey(key string) (h1, h2 uint64) {
	const (
		fnvPrime   = 1099511628211
		fnvOffset1 = 14695981039346656037
		fnvOffset2 = 14695981039346656037 ^ 0xdeadbeefcafe
	)
	h1, h2 = fnvOffset1, fnvOffset2
	for _, b := range []byte(key) {
		h1 ^= uint64(b)
		h1 *= fnvPrime
		h2 ^= uint64(b)
		h2 *= fnvPrime
	}
	return h1, h2
}
