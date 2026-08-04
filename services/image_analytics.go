package services

import (
	"fmt"
	"math"

	"github.com/disintegration/imaging"
)

// maxAnalyticsPixels caps the sample size for low-level feature extraction.
const maxAnalyticsPixels = 128 * 128

// lowLevelVectorDim is the fixed dimension of the built-in "lowlevel-v1" vector.
// Layout: 7 global + 9 cells x 4 + 8 hue bins = 51.
const lowLevelVectorDim = 51

// CellStats summarizes color statistics within one region of a 3x3 grid.
// Hue is stored as (cos, sin) so averaging does not wrap around 0/360.
type CellStats struct {
	Luma   float64 `json:"luma"`
	Sat    float64 `json:"sat"`
	HueCos float64 `json:"hue_cos"`
	HueSin float64 `json:"hue_sin"`
}

// LowLevelFeatures are continuous, analytic descriptors of image content.
type LowLevelFeatures struct {
	Vibrancy     float64          `json:"vibrancy"`     // mean saturation (0-1)
	Brightness   float64          `json:"brightness"`   // mean luma (0-1)
	Contrast     float64          `json:"contrast"`     // std-dev of luma
	SatSpread    float64          `json:"sat_spread"`   // std-dev of saturation
	Colorfulness float64          `json:"colorfulness"` // Hasler-Süsstrunk
	EdgeDensity  float64          `json:"edge_density"` // fraction of edge pixels
	Aspect       float64          `json:"aspect"`       // width/height, clamped
	Grid         [3][3]CellStats  `json:"grid"`
	HueHist      [8]float64       `json:"hue_hist"`     // normalized hue histogram
	Width        int              `json:"width"`
	Height       int              `json:"height"`
}

type pxStat struct {
	lr, lg, lb float64
	luma, sat  float64
	hue        float64 // degrees, -1 if achromatic
}

// ExtractLowLevelFeatures computes the analytic descriptors for an image file.
func ExtractLowLevelFeatures(path string) (*LowLevelFeatures, error) {
	img, err := imaging.Open(path)
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("empty image bounds")
	}

	total := w * h
	step := 1
	if total > maxAnalyticsPixels {
		step = int(math.Ceil(math.Sqrt(float64(total) / float64(maxAnalyticsPixels))))
		if step < 1 {
			step = 1
		}
	}

	var pixels []pxStat
	lumaGrid := make([]float64, 0, maxAnalyticsPixels)
	cols := 0
	rows := 0

	var (
		sumLuma, sumLuma2   float64
		sumSat, sumSat2     float64
		sumRg, sumRg2       float64
		sumYb, sumYb2       float64
		hueCount            [8]int
		cellSumLuma         [3][3]float64
		cellSumSat          [3][3]float64
		cellSumHueCos       [3][3]float64
		cellSumHueSin       [3][3]float64
		cellCount           [3][3]int
	)

	fy := float64(h)
	fx := float64(w)

	for y := 0; y < h; y += step {
		row := make([]float64, 0, 256)
		for x := 0; x < w; x += step {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			if a < 32768 {
				row = append(row, -1)
				continue
			}
			lr := float64(r>>8) / 255.0
			lg := float64(g>>8) / 255.0
			lb := float64(b>>8) / 255.0

			luma := 0.299*lr + 0.587*lg + 0.114*lb
			maxC, minC := max3f(lr, lg, lb)
			sat := 0.0
			if maxC > 0 {
				sat = (maxC - minC) / maxC
			}
			hue := rgbToHueDeg(lr, lg, lb)

			p := pxStat{lr: lr, lg: lg, lb: lb, luma: luma, sat: sat, hue: hue}
			pixels = append(pixels, p)

			sumLuma += luma
			sumLuma2 += luma * luma
			sumSat += sat
			sumSat2 += sat * sat

			rg := lr - lg
			yb := 0.5*(lr+lg) - lb
			sumRg += rg
			sumRg2 += rg * rg
			sumYb += yb
			sumYb2 += yb * yb

			if hue >= 0 {
				bin := int(hue / 45.0)
				if bin >= 8 {
					bin = 7
				}
				hueCount[bin]++
			}

			cy := int(float64(y) / fy * 3)
			cx := int(float64(x) / fx * 3)
			if cy > 2 {
				cy = 2
			}
			if cx > 2 {
				cx = 2
			}
			rad := hue * math.Pi / 180.0
			cellSumLuma[cy][cx] += luma
			cellSumSat[cy][cx] += sat
			cellSumHueCos[cy][cx] += math.Cos(rad)
			cellSumHueSin[cy][cx] += math.Sin(rad)
			cellCount[cy][cx]++

			row = append(row, luma)
		}
		if len(row) > 0 {
			if cols == 0 {
				cols = len(row)
			}
			lumaGrid = append(lumaGrid, row...)
			rows++
		}
	}

	n := len(pixels)
	if n == 0 {
		return nil, fmt.Errorf("no opaque pixels in image")
	}

	nF := float64(n)
	feat := &LowLevelFeatures{}

	feat.Brightness = sumLuma / nF
	feat.Contrast = math.Sqrt(math.Max(sumLuma2/nF-feat.Brightness*feat.Brightness, 0))
	feat.Vibrancy = sumSat / nF
	feat.SatSpread = math.Sqrt(math.Max(sumSat2/nF-feat.Vibrancy*feat.Vibrancy, 0))
	feat.Colorfulness = (math.Sqrt(math.Max(sumRg2/nF-(sumRg/nF)*(sumRg/nF), 0)) +
		math.Sqrt(math.Max(sumYb2/nF-(sumYb/nF)*(sumYb/nF), 0)) +
		0.3*math.Sqrt((sumRg/nF)*(sumRg/nF)+(sumYb/nF)*(sumYb/nF)))
	feat.Aspect = clampFloat(float64(w)/float64(h), 0.25, 4.0)
	feat.Width = w
	feat.Height = h

	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			if cellCount[r][c] > 0 {
				cnt := float64(cellCount[r][c])
				feat.Grid[r][c] = CellStats{
					Luma:   cellSumLuma[r][c] / cnt,
					Sat:    cellSumSat[r][c] / cnt,
					HueCos: cellSumHueCos[r][c] / cnt,
					HueSin: cellSumHueSin[r][c] / cnt,
				}
			}
		}
	}

	for i := 0; i < 8; i++ {
		feat.HueHist[i] = float64(hueCount[i]) / nF
	}

	feat.EdgeDensity = edgeDensity(lumaGrid, cols, rows)

	return feat, nil
}

// edgeDensity runs a cheap Sobel filter over the sampled luma grid and returns
// the fraction of interior pixels above an edge threshold.
func edgeDensity(grid []float64, cols, rows int) float64 {
	if cols < 3 || rows < 3 {
		return 0
	}
	edges := 0
	interior := 0
	for y := 1; y < rows-1; y++ {
		for x := 1; x < cols-1; x++ {
			idx := y*cols + x
			l, r, u, d := grid[idx-1], grid[idx+1], grid[idx-cols], grid[idx+cols]
			// Skip cells adjacent to transparent samples (-1 marks them)
			if l < 0 || r < 0 || u < 0 || d < 0 {
				continue
			}
			interior++
			gx := math.Abs(r - l)
			gy := math.Abs(d - u)
			if math.Sqrt(gx*gx+gy*gy) > 0.25 {
				edges++
			}
		}
	}
	if interior == 0 {
		return 0
	}
	return float64(edges) / float64(interior)
}

// ToVector renders the analytic descriptors as a fixed-size float32 vector.
// Order: [global(7) grid(36) hueHist(8)] — see lowLevelVectorDim.
func (f *LowLevelFeatures) ToVector() []float32 {
	v := make([]float32, 0, lowLevelVectorDim)
	v = append(v,
		float32(f.Vibrancy),
		float32(f.Brightness),
		float32(f.Contrast),
		float32(f.SatSpread),
		float32(f.Colorfulness/100.0),
		float32(f.EdgeDensity),
		float32(f.Aspect),
	)
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			cell := f.Grid[r][c]
			v = append(v, float32(cell.Luma), float32(cell.Sat), float32(cell.HueCos), float32(cell.HueSin))
		}
	}
	for _, b := range f.HueHist {
		v = append(v, float32(b))
	}
	if len(v) < lowLevelVectorDim {
		for len(v) < lowLevelVectorDim {
			v = append(v, 0)
		}
	}
	return v[:lowLevelVectorDim]
}

func max3f(a, b, c float64) (float64, float64) {
	max := a
	if b > max {
		max = b
	}
	if c > max {
		max = c
	}
	min := a
	if b < min {
		min = b
	}
	if c < min {
		min = c
	}
	return max, min
}

// rgbToHueDeg converts RGB (0-1 floats) to a hue in degrees [0,360), or -1
// when the color is achromatic.
func rgbToHueDeg(r, g, b float64) float64 {
	max, min := max3f(r, g, b)
	delta := max - min
	if delta < 1e-9 {
		return -1
	}
	var hue float64
	switch max {
	case r:
		hue = 60 * math.Mod((g-b)/delta, 6)
	case g:
		hue = 60 * ((b-r)/delta + 2)
	default:
		hue = 60 * ((r-g)/delta + 4)
	}
	if hue < 0 {
		hue += 360
	}
	return hue
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}