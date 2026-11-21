package services

import (
	"math"
	"testing"
)

func TestFormatThumbnailSeekArg(t *testing.T) {
	at := func(v float64) *float64 { return &v }

	tests := []struct {
		name          string
		duration      float64
		thumbnailTime *float64
		want          string
	}{
		{name: "default uses ten seconds", duration: 60, want: "10.000"},
		{name: "default short video uses one third", duration: 12, want: "4.000"},
		{name: "explicit time is preserved to milliseconds", duration: 60, thumbnailTime: at(7.1254), want: "7.125"},
		{name: "explicit time beyond duration is clamped", duration: 60, thumbnailTime: at(90), want: "59.999"},
		{name: "explicit time at duration avoids empty seek", duration: 60, thumbnailTime: at(60), want: "59.999"},
		{name: "unknown duration uses explicit time", duration: 0, thumbnailTime: at(3.5), want: "3.500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatThumbnailSeekArg(tt.duration, tt.thumbnailTime); got != tt.want {
				t.Errorf("formatThumbnailSeekArg(%v, %v) = %q, want %q", tt.duration, tt.thumbnailTime, got, tt.want)
			}
		})
	}
}

func TestResolveThumbnailSeekArgRejectsInvalidTime(t *testing.T) {
	nan := math.NaN()
	inf := math.Inf(1)
	negative := -0.001

	for _, invalid := range []float64{nan, inf, negative} {
		time := invalid
		if _, err := resolveThumbnailSeekArg("does-not-exist.mp4", &time); err == nil {
			t.Errorf("resolveThumbnailSeekArg(%v) expected validation error", invalid)
		}
	}
}
