package hlsdl

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/grafov/m3u8"
)

func parseHlsSegment(hlsUrl string, headers map[string]string) ([]*Segment, error) {
	baseUrl, err := url.Parse(hlsUrl)
	if err != nil {
		return nil, errors.New("invalid url")
	}
	p, t, err := getM3u8ListType(hlsUrl, headers)
	if err != nil {
		return nil, err
	}
	if t != m3u8.MEDIA {
		return nil, errors.New("not supported m3u8 format")
	}

	mediaPlaylist := p.(*m3u8.MediaPlaylist)
	segments := []*Segment{}

	for _, seg := range mediaPlaylist.Segments {
		if seg == nil {
			continue
		}
		if !strings.Contains(seg.URI, "http") {
			segmentUrl, err := baseUrl.Parse(seg.URI)
			if err != nil {
				return nil, err
			}
			seg.URI = segmentUrl.String()
		}
		if seg.Key == nil && mediaPlaylist.Key != nil {
			seg.Key = mediaPlaylist.Key
		}

		if seg.Key != nil && !strings.Contains(seg.Key.URI, "http") {
			keyURL, err := baseUrl.Parse(seg.Key.URI)
			if err != nil {
				return nil, err
			}
			seg.Key.URI = keyURL.String()
		}
		segment := &Segment{MediaSegment: seg}
		segments = append(segments, segment)
	}
	return segments, nil
}

func getM3u8ListType(url string, headers map[string]string) (m3u8.Playlist, m3u8.ListType, error) {
	req, err := newRequest(url, headers)
	if err != nil {
		return nil, 0, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, 0, err
	}
	play, t, err := m3u8.DecodeFrom(res.Body, false)
	if err != nil {
		return nil, 0, err
	}
	return play, t, nil
}

func newRequest(url string, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return req, err
}
