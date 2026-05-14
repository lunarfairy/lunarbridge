package transfer

import (
	"mime/multipart"
	"net/textproto"
	"testing"
)

func TestCleanBrowserRelativePath(t *testing.T) {
	header := &multipart.FileHeader{
		Filename: `folder\子目录\file.txt`,
		Header:   textproto.MIMEHeader{},
	}
	got := cleanBrowserRelativePath(header)
	want := "folder/子目录/file.txt"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
