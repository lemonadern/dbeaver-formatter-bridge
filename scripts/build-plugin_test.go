//go:build ignore

package main

import "testing"

func TestFileURI(t *testing.T) {
	for _, tt := range []struct{ path, want string }{
		{"/home/me/repo/plugin/target", "file:///home/me/repo/plugin/target"},
		{"/Users/me/SQL bridge/#target", "file:///Users/me/SQL%20bridge/%23target"},
		{`C:\Users\me\SQL bridge\target`, "file:///C:/Users/me/SQL%20bridge/target"},
		{`\\server\share\SQL bridge\target`, "file://server/share/SQL%20bridge/target"},
		{`C:\100%\日本語`, "file:///C:/100%25/%E6%97%A5%E6%9C%AC%E8%AA%9E"},
	} {
		if got := fileURI(tt.path); got != tt.want {
			t.Errorf("fileURI(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
