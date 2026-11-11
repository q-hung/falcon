package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	pb "github.com/cheggaaa/pb/v3"
	"github.com/fatih/color"
)

var client http.Client

var (
	acceptRangeHeader   = "Accept-Ranges"
	contentLengthHeader = "Content-Length"
	userAgent           = "falcon-downloader/1.0 (+https://github.com/q-hung/falcon)"
)

/*
HttpDownloader struct
*/
type HttpDownloader struct {
	url        string
	file       string
	totalParts int64
	length     int64
	parts      []Part
	resumable  bool
	// ctx is cancelled on SIGHUP/SIGINT/SIGTERM/SIGQUIT (or on the first part
	// error), so every part worker sees it, unlike a single-value signal channel.
	ctx  context.Context
	stop context.CancelFunc
}

// partResult is what each part worker reports. state is the part's resume
// state; RangeFrom > RangeTo means the part is fully downloaded.
type partResult struct {
	state    Part
	finished bool
	err      error
}

func isTextResponse(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	return strings.HasPrefix(strings.ToLower(ct), "text/") || strings.Contains(strings.ToLower(ct), "json")
}

func printResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		fmt.Printf("Server message: empty body\n")
		return
	}
	defer resp.Body.Close()
	if isTextResponse(resp) {
		limited := io.LimitReader(resp.Body, 8192)
		b, _ := io.ReadAll(limited)
		if len(b) > 0 {
			fmt.Printf("Server message: %s\n", string(b))
		}
	}
}

func probeRemote(u string) (*http.Response, error) {
	req, err := http.NewRequest("HEAD", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	if resp != nil {
		// print any text from HEAD response if provided
		printResponseBody(resp)
	}
	// Fallback for servers that don't support HEAD reliably: GET first byte
	req2, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req2.Header.Set("User-Agent", userAgent)
	req2.Header.Set("Range", "bytes=0-0")
	resp2, err := client.Do(req2)
	if err != nil {
		return nil, err
	}
	if resp2.StatusCode == http.StatusPartialContent || (resp2.StatusCode >= 200 && resp2.StatusCode < 300) {
		return resp2, nil
	}
	// print server text message then return error
	printResponseBody(resp2)
	return nil, fmt.Errorf("remote unavailable: %d %s", resp2.StatusCode, resp2.Status)
}

// NewHttpDownloader constructor
func NewHttpDownloader(url string, connections int64, parts []Part) *HttpDownloader {
	downloader := new(HttpDownloader)
	// Probe remote to ensure it exists and to obtain headers (and final URL semantics)
	resp, err := probeRemote(url)
	HandleError(err)
	defer resp.Body.Close()

	var resumable = true
	//print out host info
	downloader.printHostInfo(url)

	// CheckHTTPHeader Check if target url response
	// contains Accept-Ranges or Content-Length headers
	contentLength := resp.Header.Get(contentLengthHeader)
	acceptRange := resp.Header.Get(acceptRangeHeader)

	if contentLength == "" {
		fmt.Printf("Response header doesn't contain Content-Length, fallback to 1 connection\n")
		contentLength = "1" //set 1 because of progress bar not accept 0 length
		connections = 1
		resumable = false
	}

	if acceptRange == "" {
		fmt.Printf("Response header doesn't contain Accept-Ranges, fallback to 1 connection\n")
		connections = 1
		resumable = false
	}

	fmt.Printf("Start download with %d connections \n", connections)

	length, err := strconv.ParseInt(contentLength, 10, 64)
	HandleError(err)

	downloader.url = url
	downloader.file = FilenameFromURL(url)
	downloader.totalParts = int64(connections)
	downloader.length = length
	downloader.resumable = resumable

	if len(parts) == 0 {
		downloader.parts = calculateParts(int64(connections), length, url)
	} else {
		downloader.parts = parts
	}

	downloader.ctx, downloader.stop = signal.NotifyContext(context.Background(),
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	return downloader
}

func (d HttpDownloader) printHostInfo(url string) {
	parsed, err := neturl.Parse(url)
	HandleError(err)
	ips, err := net.LookupIP(parsed.Host)
	HandleError(err)

	ipstr := FilterIPV4(ips)
	fmt.Printf("Resolve ip: %s\n", strings.Join(ipstr, " | "))
}

func (d HttpDownloader) initProgressbars() []*pb.ProgressBar {
	bars := make([]*pb.ProgressBar, 0)
	var prefix string
	for i, part := range d.parts {
		prefix = fmt.Sprintf("%s-%d", d.file, i)
		if runtime.GOOS != "windows" {
			prefix = color.YellowString(prefix)
		}
		// Progress bar size: inclusive range, so +1
		newbar := pb.New64(max(expectedBytes(part), 1)).Set(pb.Bytes, true).Set("prefix", prefix)
		bars = append(bars, newbar)
	}
	return bars
}

func (d HttpDownloader) newRangeRequest(part Part) (*http.Request, error) {
	ranges := fmt.Sprintf("bytes=%d-%d", part.RangeFrom, part.RangeTo)
	req, err := http.NewRequestWithContext(d.ctx, "GET", part.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Add("Range", ranges)
	return req, nil
}

func (d HttpDownloader) openPartFile(part Part) (*os.File, error) {
	_, fileExists := os.Stat(part.Path)
	fileMode := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if fileExists == nil {
		fileMode = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	return os.OpenFile(part.Path, fileMode, 0600)
}

func expectedBytes(part Part) int64 {
	return part.RangeTo - part.RangeFrom + 1
}

func (d HttpDownloader) downloadPart(i int64, part Part, bar *pb.ProgressBar, wg *sync.WaitGroup) {
	defer wg.Done()
	req, err := d.newRangeRequest(part)
	d.errorChan <- err
	resp, err := client.Do(req)
	d.errorChan <- err
	if err != nil {
		return
	}
	// If non-2xx here, print any server message and abort this part
	if !(resp.StatusCode >= 200 && resp.StatusCode < 300) && resp.StatusCode != http.StatusPartialContent {
		printResponseBody(resp)
		return
	}
	defer resp.Body.Close()

	f, err := d.openPartFile(part)
	d.errorChan <- err
	defer f.Close()

	writer := bar.NewProxyWriter(f)
	expected := expectedBytes(part)

	done := make(chan int64, 1)
	errChan := make(chan error, 1)
	go func() {
		copied, err := io.Copy(writer, resp.Body)
		if err != nil {
			errChan <- err
			return
		}
		done <- copied
	}()

	var copied int64
	select {
	case <-d.signalChan:
		if fileInfo, err := os.Stat(part.Path); err == nil {
			copied = fileInfo.Size()
		}
		d.stateChan <- Part{URL: d.url, Path: part.Path, RangeFrom: copied + part.RangeFrom, RangeTo: part.RangeTo}
		return
	case err := <-errChan:
		bar.Finish()
		d.errorChan <- err
		return
	case copied = <-done:
		if copied != expected {
			bar.Finish()
			d.errorChan <- fmt.Errorf("part %d: expected %d bytes, got %d bytes", i, expected, copied)
			return
		}
		bar.Finish()
		d.fileChan <- part.Path
		return
	}
}

func (d HttpDownloader) downloadPart(i int64, part Part, bar *pb.ProgressBar) partResult {
	defer bar.Finish()
	expected := expectedBytes(part)
	if expected <= 0 { // finished in a previous run
		return partResult{state: part, finished: true}
	}

	// interrupted: part.RangeFrom advances by the bytes written in this run
	// only (counted by io.Copy, not stat'ed), so the resume offset is exact.
	interrupted := func(copied int64) partResult {
		part.RangeFrom += copied
		return partResult{state: part}
	}

	req, err := d.newRangeRequest(part)
	if err != nil {
		return partResult{err: err}
	}
	resp, err := client.Do(req)
	if err != nil {
		if d.ctx.Err() != nil {
			return interrupted(0)
		}
		return partResult{err: err}
	}
	defer resp.Body.Close()
	if !(resp.StatusCode >= 200 && resp.StatusCode < 300) && resp.StatusCode != http.StatusPartialContent {
		printResponseBody(resp)
		return partResult{err: fmt.Errorf("part %d: unexpected status %s", i, resp.Status)}
	}

	f, err := d.openPartFile(part)
	if err != nil {
		return partResult{err: err}
	}
	defer f.Close()

	// Copy on this goroutine: cancelling ctx aborts the body read, and we only
	// look at the byte count once no write is in flight.
	copied, err := io.Copy(bar.NewProxyWriter(f), resp.Body)
	if err == nil && copied == expected {
		part.RangeFrom = part.RangeTo + 1
		return partResult{state: part, finished: true}
	}
	if d.ctx.Err() != nil {
		return interrupted(copied)
	}
	if err != nil {
		return partResult{err: err}
	}
	return partResult{err: fmt.Errorf("part %d: expected %d bytes, got %d bytes", i, expected, copied)}
}

// Start downloading proccess
func (d HttpDownloader) Start() {
	defer d.stop()
	bars := d.initProgressbars()
	barPool, err := pb.StartPool(bars...)
	HandleError(err)

	// Buffered for every part, so workers never block and Start reads exactly
	// len(parts) results: there is no separate "done" signal to race with.
	results := make(chan partResult, len(d.parts))
	for i, p := range d.parts {
		go func(i int, p Part) {
			r := d.downloadPart(int64(i), p, bars[i])
			if r.err != nil {
				d.stop() // abort the sibling parts
			}
			results <- r
		}(i, p)
	}

	var (
		files    = make([]string, 0, len(d.parts))
		parts    = make([]Part, 0, len(d.parts))
		firstErr error
	)
	for range d.parts {
		r := <-results
		switch {
		case r.err != nil:
			if firstErr == nil {
				firstErr = r.err
			}
		case r.finished:
			files = append(files, r.state.Path)
			parts = append(parts, r.state)
		default:
			parts = append(parts, r.state)
		}
	}
	barPool.Stop()
	HandleError(firstErr)

	if len(files) < len(d.parts) { // interrupted
		if !d.resumable {
			fmt.Println("Interrupted, download is not resumable")
			return
		}
		fmt.Printf("Interrupted, saving state ... \n")
		HandleError((&State{URL: d.url, Parts: parts}).Save())
		return
	}

	// Check and join all parts
	if isJoinable(files) {
		HandleError(JoinFile(files, FilenameFromURL(d.url)))
	} else {
		fmt.Println("Source file is empty, no need to join")
	}
	HandleError(os.RemoveAll(GetValidFolderPath(d.url)))
}
