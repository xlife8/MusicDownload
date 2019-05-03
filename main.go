package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

/*SongInfo 存放歌曲信息的数据结构*/
type SongInfo struct {
	Src    string `json:"type"`
	Link   string `json:"link"`
	Songid string `json:"songid"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Lrc    string `json:"lrc"`
	Addr   string `json:"url"`
	Pic    string `json:"pic"`
}

/*ResInfo 存放请求的返回数据的数据结构*/
type ResInfo struct {
	Data   []SongInfo `json:"data"`
	Code   int        `json:"code"`
	ErrMsg string     `json:"error"`
}

const reqURL = "http://music.sonimei.cn"
const chanSize = 10

/*歌曲信息来源*/
var siteType = []string{"netease", "qq", "kugou", "kuwo", "xiami", "baidu", "1ting", "migu", "lizhi", "qingting", "ximalaya", "kg"}

/*GetSongInfo 获取歌曲详细信息*/
func GetSongInfo(input, filter string, resInfo *ResInfo) error {
	var err error
	var response *http.Response
	for _, value := range siteType {
		client := &http.Client{}
		params := fmt.Sprintf("input=%s&filter=%s&type=%s&page=%d", input, filter, value, 1)
		payload := strings.NewReader(params)
		request, _ := http.NewRequest("POST", reqURL, payload)
		request.Header.Set("X-Requested-With", "XMLHttpRequest")
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
		for tryDo := 0; tryDo < 3; tryDo++ {
			response, err = client.Do(request)
			if err != nil {
				if tryDo == 2 {
					fmt.Println("can not get song info,post err:", err)
					return err
				}
				time.Sleep(time.Second)
				continue
			} else {
				break
			}
		}
		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			fmt.Println("response body err:", err)
			response.Body.Close()
			continue
		}
		err = json.Unmarshal(body, resInfo)
		if err != nil {
			fmt.Println("Unmarshal response body err:", err)
			response.Body.Close()
			continue
		}
		if resInfo.Code != 200 {
			fmt.Println(value, " don't have the song,err code:", resInfo.Code)
			response.Body.Close()
			continue
		}
		return nil
	}
	err = errors.New("can not get " + input + " info")
	return err
}

/*DownLoadSongPic 下载歌曲封面图片*/
func DownLoadSongPic(savePath, songName, picURL string) error {
	var response *http.Response
	var err error
	for tryGet := 0; tryGet < 3; tryGet++ {
		response, err = http.Get(picURL)
		if err != nil {
			if tryGet == 2 {
				fmt.Println("can not get song pictrue,get err:", err)
				return err
			}
			time.Sleep(time.Second)
			continue
		} else {
			break
		}
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	file, err := os.Create(savePath + songName + ".jpg")
	if err != nil {
		fmt.Println("create song pictrue file err:", err)
		return err
	}
	writer := bufio.NewWriter(file)
	writtenSize, _ := io.Copy(writer, reader)
	if writtenSize == 0 {
		fmt.Println("failed to download song ", songName, " pictrue")
		os.Remove(savePath + songName + ".jpg")
		return errors.New("failed to download song " + songName + " pictrue")
	}
	return nil
}

/*DownLoadSong 下载歌曲(包括图片和歌词)*/
func DownLoadSong(resInfo *ResInfo, savePath string) error {

	if !strings.HasSuffix(savePath, "/") {
		savePath += "/"
	}

	song := resInfo.Data[0].Addr
	songPic := resInfo.Data[0].Pic
	songName := resInfo.Data[0].Title

	var response *http.Response
	var err error

	for tryGet := 0; tryGet < 3; tryGet++ {
		response, err = http.Get(song)
		if err != nil {
			if tryGet == 2 {
				fmt.Println("can not get song,get err:", err)
				return err
			}
			time.Sleep(time.Second)
			continue
		} else {
			break
		}
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	file, err := os.Create(savePath + songName + ".mp3")
	if err != nil {
		fmt.Println("create song file err:", err)
		return err
	}
	writer := bufio.NewWriter(file)
	writtenSize, _ := io.Copy(writer, reader)
	if writtenSize == 0 {
		fmt.Println("failed to download song ", songName)
		os.Remove(savePath + songName + ".mp3")
		return errors.New("failed to download song " + songName)
	}

	lrc, _ := os.Create(savePath + songName + ".lrc")
	lrc.WriteString(resInfo.Data[0].Lrc)

	err = DownLoadSongPic(savePath, songName, songPic)
	return err
}

/*DownLoad 封装下载过程，便于并发下载*/
func DownLoad(input, filter, savePath string, wg *sync.WaitGroup) {
	defer wg.Done()
	var err error
	fmt.Println(input, " 下载中...")
	resInfo := ResInfo{}
	err = GetSongInfo(input, filter, &resInfo)
	if err != nil {
		fmt.Println("获取 ", input, " 信息失败！")
		return
	}
	err = DownLoadSong(&resInfo, savePath)
	if err != nil {
		fmt.Println("下载 ", input, " 失败！")
		return
	}
	fmt.Println(input, " 下载成功！")
}

/*DownLoadSongFromList 并发下载控制*/
func DownLoadSongFromList(songChan chan string, wgOut *sync.WaitGroup) {
	defer wgOut.Done()
	songList, err := os.Open("./songs.txt")
	if err != nil {
		fmt.Println("read song list err:", err)
		return
	}
	defer songList.Close()

	err = os.MkdirAll("./songs", 0775)
	if err != nil {
		fmt.Println("创建songs文件夹失败，请在当前目录手动创建songs文件夹。")
		return
	}

	var wg sync.WaitGroup
	isEnd := false
	var songName string
	reader := bufio.NewReader(songList)
	for {
		line, _, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				isEnd = true
				goto createGoRoutine
			}
			fmt.Println("ReadLine err", err)
			continue
		}

		songName = string(line)
		select {
		case songChan <- songName:
			continue
		default:
			goto createGoRoutine
		}
	createGoRoutine:
		for {
			select {
			case v := <-songChan:
				wg.Add(1)
				go DownLoad(v, "name", "./songs/", &wg)
			default:
				break createGoRoutine
			}
		}
		if isEnd {
			wg.Wait()
			return
		}
		//比如管道大小为3,当管道満时第4个不能添加进去，所以要单独处理第四个
		songChan <- songName
		wg.Wait()
	}
}

func main() {
	var wg sync.WaitGroup
	songChan := make(chan string, chanSize)
	wg.Add(1)
	go DownLoadSongFromList(songChan, &wg)
	wg.Wait()
	/*
		songList, err := os.Open("./songs.txt")
		if err != nil {
			fmt.Println("read song list err:", err)
			return
		}
		defer songList.Close()

		err = os.MkdirAll("./songs", 0775)
		if err != nil {
			fmt.Println("创建songs文件夹失败，请在当前目录手动创建songs文件夹。")
			return
		}

		var wg sync.WaitGroup

		reader := bufio.NewReader(songList)
		for {
			line, _, err := reader.ReadLine()
			if err != nil {
				if err == io.EOF {
					break
				}
				fmt.Println("ReadLine err", err)
				continue
			}
			songName := string(line)
			wg.Add(1)
			go DownLoad(songName, "name", "./songs/", &wg)

		}
		wg.Wait()
	*/
}
