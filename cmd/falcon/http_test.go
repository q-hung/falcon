package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/cheggaaa/pb/v3"
)

// A cancelled part must report a resume offset equal to the bytes actually on
// disk, and must not report itself finished.
func TestDownloadPartInterruptedOffset(t *testing.T) {
	sent := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		w.Write(make([]byte, 100))
		w.(http.Flusher).Flush()
		close(sent)
		<-r.Context().Done() // never finish the body
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { <-sent; cancel() }()

	part := Part{URL: srv.URL, Path: filepath.Join(t.TempDir(), "f.part0"), RangeFrom: 0, RangeTo: 999}
	d := HttpDownloader{ctx: ctx, stop: cancel}
	r := d.downloadPart(0, part, pb.New64(1000))

	if r.err != nil || r.finished {
		t.Fatalf("want interrupted, got err=%v finished=%v", r.err, r.finished)
	}
	fi, err := os.Stat(part.Path)
	if err != nil {
		t.Fatal(err)
	}
	if r.state.RangeFrom != fi.Size() {
		t.Fatalf("resume offset %d != bytes on disk %d", r.state.RangeFrom, fi.Size())
	}
}

// A part completed in a previous run (RangeFrom > RangeTo) is not re-requested.
func TestDownloadPartAlreadyFinished(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := HttpDownloader{ctx: ctx, stop: cancel}
	r := d.downloadPart(0, Part{URL: "http://invalid.invalid", Path: "x", RangeFrom: 500, RangeTo: 499}, pb.New64(1))
	if r.err != nil || !r.finished {
		t.Fatalf("want finished, got %+v", r)
	}
}
