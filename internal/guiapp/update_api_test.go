package guiapp

import (
	"net/url"
	"reflect"
	"testing"
)

func TestIsAllowedUpdateHost(t *testing.T) {
	allowed := []string{"thrm.tianli0.top", "THRM.tianli0.top", "github.com", "api.github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com"}
	for _, host := range allowed {
		if !isAllowedUpdateHost(host) {
			t.Errorf("isAllowedUpdateHost(%q) = false, want true", host)
		}
	}
	denied := []string{"tianli0.top", "evil-thrm.tianli0.top", "github.com.evil.com", "example.com"}
	for _, host := range denied {
		if isAllowedUpdateHost(host) {
			t.Errorf("isAllowedUpdateHost(%q) = true, want false", host)
		}
	}
}

func TestUpdateDownloadCandidates(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{
			"https://thrm.tianli0.top/download/v3.7.2/THRM-amd64-installer.exe",
			[]string{
				"https://thrm.tianli0.top/download/v3.7.2/THRM-amd64-installer.exe",
				"https://github.com/TIANLI0/THRM/releases/download/v3.7.2/THRM-amd64-installer.exe",
			},
		},
		{
			"https://thrm.tianli0.top/download/latest/THRM-amd64-installer.exe",
			[]string{
				"https://thrm.tianli0.top/download/latest/THRM-amd64-installer.exe",
				"https://github.com/TIANLI0/THRM/releases/latest/download/THRM-amd64-installer.exe",
			},
		},
		{
			// 已经是 GitHub 直链时不再追加
			"https://github.com/TIANLI0/THRM/releases/download/v3.7.2/THRM-amd64-installer.exe",
			[]string{"https://github.com/TIANLI0/THRM/releases/download/v3.7.2/THRM-amd64-installer.exe"},
		},
		{
			// 官网上不是 /download/<标签>/<文件> 形式的地址，不猜测 GitHub 地址
			"https://thrm.tianli0.top/changelog/",
			[]string{"https://thrm.tianli0.top/changelog/"},
		},
	}
	for _, c := range cases {
		u, err := url.Parse(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if got := updateDownloadCandidates(u); !reflect.DeepEqual(got, c.want) {
			t.Errorf("updateDownloadCandidates(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
