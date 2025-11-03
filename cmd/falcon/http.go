package main

import (
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
	"sync"
	"syscall"

	pb "github.com/cheggaaa/pb/v3"
	"github.com/fatih/color"
)

var (
	client http.Client
	err    error
)

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
	fileChan   chan string
	doneChan   chan bool
	errorChan  chan error
	signalChan chan os.Signal
	stateChan  chan Part
}

// NewHttpDownloader constructor
func NewHttpDownloader(url string, connections int64, parts []Part) *HttpDownloader {
	downloader := new(HttpDownloader)
	header := downloader.getHeader(url)
	var resumable = true
	//print out host info
	downloader.printHostInfo(url)

	// CheckHTTPHeader Check if target url response
	// contains Accept-Ranges or Content-Length headers
	contentLength := header.Get(contentLengthHeader)
	acceptRange := header.Get(acceptRangeHeader)

	if contentLength == "" {
		fmt.Printf("Response header doesn't contain Content-Length, fallback to 1 connection\n")
		contentLength = "1" //set 1 because of progress bar not accept 0 length
		connections = 1
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

	downloader.fileChan = make(chan string, int64(connections))
	downloader.doneChan = make(chan bool, int64(connections))
	downloader.errorChan = make(chan error, 1)
	downloader.stateChan = make(chan Part, 1)
	downloader.signalChan = make(chan os.Signal, 1)

	signal.Notify(downloader.signalChan,
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

func (d HttpDownloader) getHeader(url string) *http.Header {
	if !IsValidURL(url) {
		fmt.Printf("Invalid url\n")
		os.Exit(1)
	}

	req, err := http.NewRequest("GET", url, nil)
	HandleError(err)

	// Set User-Agent to respect server policies
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	HandleError(err)

	return &resp.Header
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
		newbar := pb.New64(part.RangeTo-part.RangeFrom+1).Set(pb.Bytes, true).Set("prefix", prefix)
		bars = append(bars, newbar)
	}
	return bars
}

func (d HttpDownloader) newRangeRequest(part Part) (*http.Request, error) {
	ranges := fmt.Sprintf("bytes=%d-%d", part.RangeFrom, part.RangeTo)
	req, err := http.NewRequest("GET", part.URL, nil)
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

// Start downloading proccess
func (d HttpDownloader) Start() {
	var (
		files      = make([]string, 0)
		parts      = make([]Part, 0)
		interupted = false
		filename   = FilenameFromURL(d.url)
	)

	go d.download()

	for {
		select {
		case file := <-d.fileChan:
			files = append(files, file)
		case err := <-d.errorChan:
			HandleError(err)
		case part := <-d.stateChan:
			parts = append(parts, part)
			interupted = true
		case <-d.doneChan:
			if interupted && d.resumable {
				fmt.Printf("Interrupted, saving state ... \n")
				s := &State{URL: d.url, Parts: parts}
				err = s.Save()
				HandleError(err)
				return
			}
			// Check and join all parts
			if isJoinable(files) {
				err = JoinFile(files, filename)
				HandleError(err)
			} else {
				fmt.Println("Source file is empty, no need to join")
			}
			err = os.RemoveAll(GetValidFolderPath(d.url))
			HandleError(err)
			return
		}
	}
}

func (d HttpDownloader) download() {
	var (
		ws      sync.WaitGroup
		barPool *pb.Pool
	)
	bars := d.initProgressbars()
	barPool, err = pb.StartPool(bars...)
	d.errorChan <- err
	defer barPool.Stop()

	for i, p := range d.parts {
		ws.Add(1)
		go d.downloadPart(int64(i), p, bars[i], &ws)
	} //end for
	ws.Wait()
	d.doneChan <- true
}
