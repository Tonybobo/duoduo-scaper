package hlsdl

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/grafov/m3u8"
	"gopkg.in/cheggaaa/pb.v1"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

type HlsDL struct {
	client    *http.Client
	headers   map[string]string
	dir       string
	hlsURL    string
	workers   int
	bar       *pb.ProgressBar
	enableBar bool
}

type Segment struct {
	*m3u8.MediaSegment
	Path string
}

type DownloadResult struct {
	Err   error
	SeqId uint64
}

func New(hlsURL string, headers map[string]string, dir string, workers int, enableBar bool) *HlsDL {
	hlsdl := &HlsDL{
		client:    &http.Client{},
		headers:   headers,
		dir:       dir,
		hlsURL:    hlsURL,
		workers:   workers,
		enableBar: enableBar,
	}
	return hlsdl
}

func wait(wg *sync.WaitGroup) chan bool {
	c := make(chan bool, 1)
	go func() {
		wg.Wait()
		c <- true
	}()
	return c
}

func (hls *HlsDL) downloadSegments(segments []*Segment) error {
	wg := &sync.WaitGroup{}
	wg.Add(hls.workers)
	finishedChan := wait(wg)
	quitChan := make(chan bool)
	segmentChan := make(chan *Segment)
	downloadResultChan := make(chan *DownloadResult, hls.workers)

	for i := 0; i < hls.workers; i++ {
		go func() {
			defer wg.Done()

			for segment := range segmentChan {
				tried := 0
			DOWNLOAD:
				tried++

				select {
				case <-quitChan:
					return
				default:
				}

				if err := hls.downloadSegment(segment); err != nil {
					if strings.Contains(err.Error(), "connection reset by peer") && tried < 3 {
						time.Sleep(time.Second)
						log.Println("retry download segment", segment.SeqId)
						goto DOWNLOAD
					}
					downloadResultChan <- &DownloadResult{Err: err, SeqId: segment.SeqId}
					return
				}
				downloadResultChan <- &DownloadResult{SeqId: segment.SeqId}
			}
		}()
	}

	go func() {
		defer close(segmentChan)

		for _, segment := range segments {
			segName := fmt.Sprintf("seg%d.ts", segment.SeqId)
			segment.Path = filepath.Join(hls.dir, segName)

			select {
			case segmentChan <- segment:
			case <-quitChan:
				return
			}
		}
	}()
	if hls.enableBar {
		hls.bar = pb.New(len(segments)).SetMaxWidth(100).Prefix("Downloading ...")
		hls.bar.ShowElapsedTime = true
		hls.bar.Start()
	}

	defer func() {
		if hls.enableBar {
			hls.bar.Finish()
		}
	}()

	for {
		select {
		case <-finishedChan:
			return nil
		case result := <-downloadResultChan:
			if result.Err != nil {
				close(quitChan)
				return result.Err
			}
			if hls.enableBar {
				hls.bar.Increment()
			}
		}
	}
}

func (hls *HlsDL) downloadSegment(segment *Segment) error {
	req, err := newRequest(segment.URI, hls.headers)
	if err != nil {
		return err
	}
	res, err := hls.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return errors.New(res.Status)
	}

	file, err := os.Create(segment.Path)

	if err != nil {
		return err
	}

	defer file.Close()

	if _, err := io.Copy(file, res.Body); err != nil {
		return err
	}

	return nil
}

func (hls *HlsDL) Download() (string, error) {
	segs, err := parseHlsSegment(hls.hlsURL, hls.headers)

	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(hls.dir, os.ModePerm); err != nil {
		return "", err
	}
	if err := hls.downloadSegments(segs); err != nil {
		return "", err
	}
	return "", nil
}
