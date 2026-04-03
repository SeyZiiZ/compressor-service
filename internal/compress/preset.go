package compress

type VideoPreset struct {
	Codec    string
	CRF      int
	MaxWidth int
	Format   string
	Extra    []string // additional FFmpeg flags
}

type ImagePreset struct {
	Quality int
	Width   int
	Height  int
	Format  string
}

var VideoPresets = map[string]VideoPreset{
	"web-optimized": {
		Codec:    "libx264",
		CRF:      23,
		MaxWidth: 1920,
		Format:   "mp4",
		Extra:    []string{"-preset", "medium", "-movflags", "+faststart"},
	},
	"mobile": {
		Codec:    "libx264",
		CRF:      28,
		MaxWidth: 1280,
		Format:   "mp4",
		Extra:    []string{"-preset", "fast", "-movflags", "+faststart"},
	},
	"archive": {
		Codec:    "libx264",
		CRF:      18,
		MaxWidth: 0, // no resize
		Format:   "mp4",
		Extra:    []string{"-preset", "slow"},
	},
	"h265-efficient": {
		Codec:    "libx265",
		CRF:      28,
		MaxWidth: 1920,
		Format:   "mp4",
		Extra:    []string{"-preset", "medium", "-tag:v", "hvc1"},
	},
}

var ImagePresets = map[string]ImagePreset{
	"web-optimized": {Quality: 80, Width: 1920, Height: 0, Format: "webp"},
	"thumbnail":     {Quality: 70, Width: 320, Height: 320, Format: "jpeg"},
	"mobile":        {Quality: 75, Width: 1280, Height: 0, Format: "webp"},
	"archive":       {Quality: 95, Width: 0, Height: 0, Format: "png"},
}
