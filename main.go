package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/tonybobo/duoduo-downloader/hlsdl"
)

func main() {
	var text string
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Please Provide The Exact Movie Name")
	fmt.Println("---------------------")
	for {
		text, _ = reader.ReadString('\n')
		if text != "" {
			break
		}
	}
	text = strings.Trim(text, "\r\n")
	list, _ := getM3u8Url(text)
	for i := 0; i < len(list); i++ {
		hlsDL := hlsdl.New(list[i], nil, "download", 10, true, fmt.Sprintf("%s %d", text, i))
		_, err := retry(hlsDL)
		if err != nil {
			fmt.Printf("%s cannot be downloaded", list[i])
		}
	}

}

func retry(hlsDL *hlsdl.HlsDL) (string, error) {
	for i := 0; i < 10; i++ {
		filepath, err := hlsDL.Download()
		if i > 0 {
			os.RemoveAll("download")
			fmt.Printf("retrying after error %s\n", err)
			time.Sleep(time.Second * 5)
		}
		if err == nil {
			return fmt.Sprintf("video has been downloaded%s ", filepath), nil
		}
	}
	return "", errors.New("unable to download video after fifth tries")
}

func getM3u8Url(text string) ([]string, error) {
	var list []string
	url := fmt.Sprintf("https://ddzyz1.com/vodsearch/-------------.html?wd=%s&submit=search", text)
	res, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatal(err)
	}
	title, exist := doc.Find(".xing_vb ul .xing_vb4 a").Attr("href")
	if !exist {
		fmt.Printf("No Movie with such name has been found")
		return list, errors.New("no movie with such name has been found")
	}

	videoUrl := fmt.Sprintf("https://ddzyz1.com%s", title)
	videoRes, err := http.Get(videoUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer videoRes.Body.Close()
	videoDoc, err := goquery.NewDocumentFromReader(videoRes.Body)
	if err != nil {
		log.Fatal(err)
	}
	untrimString := videoDoc.Find(".vodplayinfo ul li").Text()
	m3u8 := strings.Split(strings.Split(untrimString, "$")[1], " ")[0]
	if m3u8 == "" {
		fmt.Print("no available files")
		return list, errors.New("no available files")
	}
	list = append(list, m3u8)
	return list, nil
}
