package web

import "testing"

func TestYouTubeID(t *testing.T) {
	tests := map[string]string{
		"dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":  "dQw4w9WgXcQ",
		"https://example.com/watch?v=dQw4w9WgXcQ":     "",
		"not-a-valid-video":                           "",
	}
	for input, want := range tests {
		if got := youtubeID(input); got != want {
			t.Fatalf("youtubeID(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestVimeoID(t *testing.T) {
	tests := map[string]string{
		"76979871":                                "76979871",
		"https://vimeo.com/76979871":              "76979871",
		"https://player.vimeo.com/video/76979871": "76979871",
		"https://example.com/video/76979871":      "",
		"https://vimeo.com/not-a-number":          "",
	}
	for input, want := range tests {
		if got := vimeoID(input); got != want {
			t.Fatalf("vimeoID(%q)=%q, want %q", input, got, want)
		}
	}
}
