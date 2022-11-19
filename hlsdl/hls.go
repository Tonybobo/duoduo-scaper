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
	"sort"
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
	title     string
}

type Segment struct {
	*m3u8.MediaSegment
	Path string
}

type DownloadResult struct {
	Err   error
	SeqId uint64
}

func New(hlsURL string, headers map[string]string, dir string, workers int, enableBar bool, title string) *HlsDL {
	hlsdl := &HlsDL{
		client:    &http.Client{},
		headers:   headers,
		dir:       dir,
		hlsURL:    hlsURL,
		workers:   workers,
		enableBar: enableBar,
		title:     title,
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

	//send segments to segment channel
	go func() {
		defer close(segmentChan)

		for _, segment := range segments {
			segName := fmt.Sprintf("%s%d.ts", hls.title, segment.SeqId)
			segment.Path = filepath.Join(hls.dir, segName)

			select {
			case segmentChan <- segment:
			case <-quitChan:
				return
			}
		}
	}()

	// consume segments in segment channel
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

	filepath, err := hls.join(hls.dir, segs)
	if err != nil {
		return "", err
	}
	return filepath, nil
}

func (hls *HlsDL) join(dir string, segments []*Segment) (string, error) {
	fmt.Println("Joining Segments")

	filepath := filepath.Join(dir, fmt.Sprintf("%s.ts", hls.title))

	file, err := os.Create(filepath)

	if err != nil {
		return "", err
	}

	defer file.Close()

	sort.Slice(segments, func(i, j int) bool {
		return segments[i].SeqId < segments[j].SeqId
	})

	for _, segment := range segments {
		d, err := hls.decrypt(segment)

		if err != nil {
			return "", err
		}

		if _, err := file.Write(d); err != nil {
			return "", err
		}
		if err := os.RemoveAll(segment.Path); err != nil {
			return "", err
		}
	}
	return filepath, nil

}
