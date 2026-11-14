package player

import (
	"math"
	"math/cmplx"
	"sync"
	"sync/atomic"
)

// vizRingSize holds enough mono frames for the largest analysis window plus the
// ALSA buffer lag at 192 kHz.
const vizRingSize = 1 << 15

// vizTap is a read-only side channel on the samples handed to ALSA. It never
// touches the PCM going to the device, so bit-perfect output is unaffected.
type vizTap struct {
	mu   sync.Mutex
	ring [vizRingSize]float32
	head int // next write index
	fill int

	rate uint32 // atomic
	lag  int64  // atomic: frames between "written to ALSA" and "audible"
}

// push appends interleaved S32LE frames, downmixed to mono.
func (v *vizTap) push(data []byte, frames, channels, bps int) {
	if bps != 4 || channels <= 0 || frames <= 0 {
		return
	}
	stride := channels * bps
	if len(data) < frames*stride {
		frames = len(data) / stride
	}
	inv := float32(1.0 / (float64(channels) * 2147483648.0))
	v.mu.Lock()
	for f := 0; f < frames; f++ {
		var sum float32
		base := f * stride
		for c := 0; c < channels; c++ {
			o := base + c*4
			s := int32(uint32(data[o]) | uint32(data[o+1])<<8 | uint32(data[o+2])<<16 | uint32(data[o+3])<<24)
			sum += float32(s)
		}
		v.ring[v.head] = sum * inv
		v.head = (v.head + 1) & (vizRingSize - 1)
	}
	v.fill = min(v.fill+frames, vizRingSize)
	v.mu.Unlock()
}

func (v *vizTap) reset() {
	v.mu.Lock()
	v.fill = 0
	v.mu.Unlock()
}

// Spectrum returns bands log-spaced magnitudes in 0..1 for the audio that is
// currently audible (compensating for the ALSA buffer). Returns nil when no
// audio has been seen yet.
func (p *Player) Spectrum(bands int) []float64 {
	v := &p.viz
	rate := int(atomic.LoadUint32(&v.rate))
	if rate <= 0 || bands <= 0 {
		return nil
	}
	// ~90 ms window regardless of sample rate.
	n := 1024
	for n < rate*90/1000 && n < vizRingSize/2 {
		n <<= 1
	}
	lag := int(atomic.LoadInt64(&v.lag))
	lag = min(max(lag, 0), vizRingSize-n)

	buf := make([]complex128, n)
	v.mu.Lock()
	if v.fill < n+lag {
		v.mu.Unlock()
		return nil
	}
	start := (v.head - lag - n + 2*vizRingSize) & (vizRingSize - 1)
	for i := 0; i < n; i++ {
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1)) // Hann
		buf[i] = complex(float64(v.ring[(start+i)&(vizRingSize-1)])*w, 0)
	}
	v.mu.Unlock()

	fft(buf)

	out := make([]float64, bands)
	const fLo, fHi = 35.0, 16000.0
	hi := min(fHi, float64(rate)/2*0.95)
	binHz := float64(rate) / float64(n)
	for b := 0; b < bands; b++ {
		f0 := fLo * math.Pow(hi/fLo, float64(b)/float64(bands))
		f1 := fLo * math.Pow(hi/fLo, float64(b+1)/float64(bands))
		i0 := int(f0 / binHz)
		i1 := max(int(f1/binHz), i0+1)
		var peak float64
		for i := i0; i < i1 && i < n/2; i++ {
			peak = max(peak, cmplx.Abs(buf[i]))
		}
		// Normalise for window gain, tilt up highs (music is ~pink), map dB.
		mag := peak / (float64(n) / 4)
		mag *= math.Sqrt(math.Sqrt(f0 / 100))
		db := 20 * math.Log10(mag+1e-9)
		out[b] = min(max((db+60)/54, 0), 1)
	}
	return out
}

// fft is an in-place iterative radix-2 Cooley–Tukey FFT; len(a) must be a power of 2.
func fft(a []complex128) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		ang := -2 * math.Pi / float64(size)
		wl := complex(math.Cos(ang), math.Sin(ang))
		for i := 0; i < n; i += size {
			w := complex(1, 0)
			for k := 0; k < size/2; k++ {
				u := a[i+k]
				t := w * a[i+k+size/2]
				a[i+k] = u + t
				a[i+k+size/2] = u - t
				w *= wl
			}
		}
	}
}
