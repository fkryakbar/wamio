package whatsapp

import "testing"

func TestClassifyAttachment(t *testing.T) {
	cases := []struct {
		requested string
		mimetype  string
		want      string
		wantErr   bool
	}{
		{"media", "image/jpeg", "image", false},
		{"media", "video/mp4", "video", false},
		{"media", "application/pdf", "", true},
		{"audio", "audio/ogg", "audio", false},
		{"audio", "image/png", "", true},
		{"document", "image/png", "document", false},
		{"auto", "application/pdf", "document", false},
	}
	for _, tc := range cases {
		got, err := classifyAttachment(tc.requested, tc.mimetype)
		if tc.wantErr && err == nil {
			t.Errorf("classifyAttachment(%q, %q) expected error", tc.requested, tc.mimetype)
		}
		if !tc.wantErr && (err != nil || got != tc.want) {
			t.Errorf("classifyAttachment(%q, %q) = %q, %v; want %q", tc.requested, tc.mimetype, got, err, tc.want)
		}
	}
}
