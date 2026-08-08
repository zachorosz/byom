package storage

import "testing"

func TestLocationRoot(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    string
		wantErr bool
	}{
		{name: "FileURI", uri: "file:///mnt/music/Library", want: "/mnt/music/Library"},
		{name: "FileURILocalhost", uri: "file://localhost/mnt/music", want: "/mnt/music"},
		{name: "FileURIEscaped", uri: "file:///mnt/My%20Music", want: "/mnt/My Music"},
		{name: "BarePath", uri: "/mnt/music/Library", want: "/mnt/music/Library"},
		{name: "RemoteHost", uri: "file://nas/music", wantErr: true},
		{name: "UnsupportedScheme", uri: "https://example.com/music", wantErr: true},
		{name: "EmptyFilePath", uri: "file://", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := Location{URI: tt.uri}
			got, err := st.Root()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Location{URI: %q}.Root() = %q, want error", tt.uri, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Location{URI: %q}.Root() failed: %v", tt.uri, err)
			}
			if got != tt.want {
				t.Errorf("Location{URI: %q}.Root() = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}
