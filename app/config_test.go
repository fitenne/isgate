package app_test

import (
	"crypto/rand"
	"encoding/base64"
	"isgate/app"
	"testing"

	"go.yaml.in/yaml/v4"
)

func rand32Bytes() []byte {
	b := make([]byte, 32)
	rand.Read(b)
	return b
}

func TestB64Key_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		v       []byte
		wantErr bool
	}{
		{
			name:    "invalid",
			v:       []byte("invalid"),
			wantErr: true,
		},
		{
			name:    "too short",
			v:       []byte(base64.StdEncoding.EncodeToString([]byte("01234567890123456789"))),
			wantErr: true,
		},
		{
			name:    "normal",
			v:       []byte(base64.StdEncoding.EncodeToString(rand32Bytes())),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var k app.B64Key
			gotErr := yaml.Load([]byte(tt.v), &k)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("UnmarshalYAML() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("UnmarshalYAML() succeeded unexpectedly")
			}
		})
	}
}
