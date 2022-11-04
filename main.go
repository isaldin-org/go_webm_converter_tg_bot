package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/boltdb/bolt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var allowedChatId, _ = strconv.ParseInt(os.Getenv("ALLOWED_CHAT_ID"), 0, 64)
var db, _ = bolt.Open("boltdb_files/webms.db", 0600, nil)
var webmUrlBucketName = "url_to_file_id"
var urlChan = make(chan *tgbotapi.Update)

func main() {
	defer db.Close()

	err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte("url_to_file_id")); err != nil {
			log.Fatal("Db error", err)
			return err
		}
		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	bot, err := tgbotapi.NewBotAPI(getBotToken())
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = os.Getenv("DEBUG") == "true"

	log.Println("Bot started", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 5

	updates := bot.GetUpdatesChan(u)

	go listenUrls(bot)

	for update := range updates {
		if update.Message == nil || update.Message.Chat.ID != allowedChatId {
			continue
		}

		url := update.Message.Text

		if !isWebmUrl(url) {
			log.Printf("Got url %s, but not webm", url)
			continue
		}

		fileId, _ := getFileIDForUrl(url)
		if fileId != "" {
			log.Printf("File id for url %s already exists", url)
			err := sendFromFileID(bot, &update, fileId)
			if err != nil {
				responseWithError(bot, &update, err)
			}
			continue
		}

		log.Printf("Has been added url to queue: [%s]", url)
		urlChan <- &update
		continue
	}
}

func listenUrls(bot *tgbotapi.BotAPI) {
	for {
		update := <-urlChan
		url := update.Message.Text

		if fileId, _ := getFileIDForUrl(url); fileId != "" {
			sendFromFileID(bot, update, fileId)
			continue
		}

		log.Printf("Start proccessing url [%s]", url)

		if _, err := isUrlSuitableForConvertation(url); err != nil {
			log.Printf("Seems that file with url [%s] is not webm", url)
			log.Println("Related error", err)
			responseWithError(bot, update, err)
			continue
		}

		_, err := downloadConvertAndSend(bot, update, url)
		if err != nil {
			log.Println("Something going wrong while file converting", err)
			responseWithError(bot, update, err)
			continue
		}
	}
}

func sendFromFileID(bot *tgbotapi.BotAPI, update *tgbotapi.Update, fileId string) error {
	msg := tgbotapi.NewVideo(update.Message.Chat.ID, tgbotapi.FileID(fileId))
	msg.ReplyToMessageID = update.Message.MessageID
	_, err := bot.Send(msg)

	return err
}

func isWebmUrl(url string) bool {
	match, err := regexp.Match("^http(s)?:\\/\\/[a-zA-Z0-9]{2,256}\\.[a-z]{2,6}\\/.*\\.webm$", []byte(url))

	if err != nil {
		return false
	}

	return match
}

func getBotToken() string {
	return os.Getenv("TOKEN")
}

func isUrlSuitableForConvertation(url string) (bool, error) {
	resp, err := http.Head(url)
	if err != nil {
		return false, errors.New("failed to receive file headers from url")
	}

	if resp.StatusCode != http.StatusOK {
		return false, errors.New("status code is not OK")
	}

	fileType := resp.Header.Get("Content-Type")
	if fileType != "video/webm" {
		return false, errors.New("content type is not suitable to webm")
	}

	size, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	if size > 50*1024*1024 {
		return false, errors.New("size should less than 50mb")
	}

	return true, nil
}

func downloadConvertAndSend(bot *tgbotapi.BotAPI, update *tgbotapi.Update, url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		log.Println(err)
		return "", errors.New("download file error")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("download file error")
	}
	log.Println("File available for downloading")

	file, err := os.Create("temp.webm")
	if err != nil {
		log.Println(err)
		return "", err
	}

	defer file.Close()
	defer os.Remove("temp.webm")

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		log.Println(err)
		return "", err
	}
	log.Println("File downloaded")

	cmd := exec.Command("ffmpeg", "-i", "temp.webm", "-crf", "26", "video.mp4")
	defer os.Remove("video.mp4")

	log.Println("Start converting")
	if err := cmd.Run(); err != nil {
		log.Println(err)
		return "", err
	}
	log.Println("Converting has done")

	videoFile, err := os.ReadFile("video.mp4")
	if err != nil {
		return "", err
	}

	video := tgbotapi.NewVideo(update.Message.Chat.ID, tgbotapi.FileBytes{
		Name:  "converted webm",
		Bytes: videoFile,
	})
	video.ReplyToMessageID = update.Message.MessageID

	log.Println("Sending video")

	sendVideo, err := bot.Send(video)
	if err != nil {
		return "", err
	}

	log.Println("Video sent")

	db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(webmUrlBucketName))
		return b.Put([]byte(url), []byte(sendVideo.Video.FileID))
	})

	return "success", nil
}

func responseWithError(bot *tgbotapi.BotAPI, update *tgbotapi.Update, err error) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
	msg.ReplyToMessageID = update.Message.MessageID
	if _, err := bot.Send(msg); err != nil {
		log.Println("Some error", err)
	}
}

func getFileIDForUrl(url string) (string, error) {
	var fileId = ""
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(webmUrlBucketName))
		v := b.Get([]byte(url))
		fileId = string(v)
		return nil
	})

	return fileId, err
}
