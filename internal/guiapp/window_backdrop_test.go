package guiapp

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestBackdropTypeForMode(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		windows11 bool
		want      application.BackdropType
	}{
		{name: "auto on Windows 11", mode: "auto", windows11: true, want: application.Mica},
		{name: "auto on Windows 10", mode: "auto", windows11: false, want: application.None},
		{name: "legacy on", mode: "on", windows11: true, want: application.Mica},
		{name: "acrylic", mode: "acrylic", windows11: true, want: application.Acrylic},
		{name: "mica", mode: "mica", windows11: true, want: application.Mica},
		{name: "mica alt", mode: "tabbed", windows11: true, want: application.Tabbed},
		{name: "off", mode: "off", windows11: true, want: application.None},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backdropTypeForMode(tt.mode, tt.windows11); got != tt.want {
				t.Fatalf("backdropTypeForMode(%q, %v) = %v, want %v", tt.mode, tt.windows11, got, tt.want)
			}
		})
	}
}
