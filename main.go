package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"

	"github.com/boltdb/bolt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	allowedChatId, _  = strconv.ParseInt(os.Getenv("ALLOWED_CHAT_ID"), 0, 64)
	db, dbErr         = bolt.Open("boltdb_files/webms_checksums.db", 0600, nil)
	webmUrlBucketName = "url_to_file_id"
	urlChan           = make(chan WebmMessage)
)

type WebmMessage struct {
	message   string
	messageId int
	chatId    int64
}

func main() {
	if dbErr != nil {
		log.Panic("DB opening error")
	}
	defer db.Close()

	err := initDB()
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

		log.Printf("Has been added url to queue: [%s]", url)
		urlChan <- WebmMessage{
			message:   update.Message.Text,
			messageId: update.Message.MessageID,
			chatId:    update.Message.Chat.ID,
		}
		continue
	}
}

func listenUrls(bot *tgbotapi.BotAPI) {
	for {
		update := <-urlChan
		url := update.message

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

func downloadConvertAndSend(bot *tgbotapi.BotAPI, messageStruct WebmMessage, url string) (string, error) {
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

	fileHash, err := getFileHash()
	if err == nil {
		fileId, err := fileIdByChecksum(fileHash)
		if err == nil {
			log.Println("Already sent. Return by FileID")
			video := tgbotapi.NewVideo(int64(messageStruct.chatId), tgbotapi.FileID(fileId))
			video.ReplyToMessageID = messageStruct.messageId
			if _, err := bot.Send(video); err != nil {
				log.Println("Something going wrong when send video")
				return "", err
			}
			return "success", nil
		}
	}

	cmd := exec.Command("ffmpeg", "-i", "temp.webm", "-crf", "26", "video.mp4")
	defer os.Remove("video.mp4")

	log.Println("Start converting")
	if err := cmd.Run(); err != nil {
		log.Println(err)
		return "", errors.New("trouble with ffmpeg. Niabissuy")
	}
	log.Println("Converting has done")

	videoFile, err := os.ReadFile("video.mp4")
	if err != nil {
		return "", err
	}

	video := tgbotapi.NewVideo(messageStruct.chatId, tgbotapi.FileBytes{
		Name:  "converted webm",
		Bytes: videoFile,
	})
	video.ReplyToMessageID = messageStruct.messageId

	log.Println("Sending video")

	sendVideo, err := bot.Send(video)
	if err != nil {
		return "", err
	}

	log.Println("Video sent")

	db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(webmUrlBucketName))
		return b.Put([]byte(fileHash), []byte(sendVideo.Video.FileID))
	})

	return "success", nil
}

func responseWithError(bot *tgbotapi.BotAPI, messageStruct WebmMessage, err error) {
	msg := tgbotapi.NewMessage(messageStruct.chatId, err.Error())
	msg.ReplyToMessageID = messageStruct.messageId
	if _, err := bot.Send(msg); err != nil {
		log.Println("Some error", err)
	}
}

func initDB() error {
	err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte("url_to_file_id")); err != nil {
			log.Fatal("Db error", err)
			return err
		}
		return nil
	})

	return err
}

func getBotToken() string {
	return os.Getenv("TOKEN")
}

func fileIdByChecksum(checksum string) (string, error) {
	fileId := ""

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(webmUrlBucketName))
		fileId = string(b.Get([]byte(checksum)))

		return nil
	})

	if err != nil {
		return fileId, err
	}

	if fileId == "" {
		return fileId, errors.New("not found")
	}

	return fileId, err
}
