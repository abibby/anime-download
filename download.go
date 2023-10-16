package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/abibby/transmissionrpc"
	"github.com/mmcdole/gofeed"
	bolt "go.etcd.io/bbolt"
)

type Download struct {
	ID               []byte
	TorrentURL       string
	Compleated       bool
	Series           string
	TransmissionHash string
}

func download(db *bolt.DB, transmissionbt *transmissionrpc.Client, s *Series) error {

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(s.RSS)
	if err != nil {
		return err
	}

	err = initBucket(db, bucket)
	if err != nil {
		return err
	}

	re := regexp.MustCompile(s.RegExp)

	for _, item := range feed.Items {
		if s.RegExp != "" && re.FindString(item.Title) == "" {
			continue
		}
		log.Printf("found episode %s", item.Title)
		id := []byte(item.Link)

		err = db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(bucket)
			if b.Get(id) != nil {
				return nil
			}
			log.Printf("Downloading %s", item.Title)

			file := ""
			if len(item.Enclosures) > 0 {
				file = item.Enclosures[0].URL
			} else if item.Link != "" {
				file = item.Link
			}

			path := "/tmp/" + file
			err := os.MkdirAll(filepath.Dir(path), 0755)
			if err != nil {
				return err
			}
			err = downloadFile(path, file)
			if err != nil {
				return err
			}

			torrent, err := transmissionbt.TorrentAddFile(path)
			// torrent, err := transmissionbt.TorrentAdd(&transmissionrpc.TorrentAddPayload{
			// 	Filename: &path,
			// })
			if err != nil {
				return err
			}

			d := &Download{
				ID:               id,
				Series:           s.Title,
				TorrentURL:       item.Link,
				Compleated:       false,
				TransmissionHash: *torrent.HashString,
			}
			by, err := json.Marshal(d)
			if err != nil {
				return err
			}
			err = b.Put(id, by)
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			log.Print(err)
		}
	}

	return nil
}
