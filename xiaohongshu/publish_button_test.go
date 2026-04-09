package xiaohongshu

import "testing"

func TestButtonTextMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		text       string
		candidates []string
		want       bool
	}{
		{
			name:       "match publish button",
			text:       "\u53d1\u5e03",
			candidates: []string{"\u53d1\u5e03"},
			want:       true,
		},
		{
			name:       "match draft button with whitespace",
			text:       " \u5b58\u8349\u7a3f \n",
			candidates: []string{"\u5b58\u8349\u7a3f"},
			want:       true,
		},
		{
			name:       "match alternate draft text",
			text:       "\u4fdd\u5b58\u8349\u7a3f",
			candidates: []string{"\u5b58\u8349\u7a3f", "\u4fdd\u5b58\u8349\u7a3f"},
			want:       true,
		},
		{
			name:       "match save and leave draft text",
			text:       "\u6682\u5b58\u79bb\u5f00",
			candidates: []string{"\u6682\u5b58\u79bb\u5f00", "\u6682\u5b58"},
			want:       true,
		},
		{
			name:       "no match",
			text:       "\u53d1\u5e03",
			candidates: []string{"\u5b58\u8349\u7a3f"},
			want:       false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buttonTextMatches(tt.text, tt.candidates)
			if got != tt.want {
				t.Fatalf("buttonTextMatches(%q, %v) = %v, want %v", tt.text, tt.candidates, got, tt.want)
			}
		})
	}
}
