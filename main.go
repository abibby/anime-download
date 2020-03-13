package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"

	"github.com/hekmon/transmissionrpc"
	"github.com/mmcdole/gofeed"
	bolt "go.etcd.io/bbolt"
)

type Download struct {
	ID               []byte
	TorrentURL       string
	Downloading      bool
	Series           string
	TransmissionHash string
}

type Series struct {
	Title  string `yaml:"title"`
	RSS    string `yaml:"rss"`
	RegExp string `yaml:"regexp"`
}

type Config struct {
	Connection string    `yaml:"connection"`
	Series     []*Series `yaml:"series"`
}

var bucket = []byte("MyBucket")

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func btClient(connection string) (*transmissionrpc.Client, error) {
	u, err := url.Parse(connection)
	check(err)

	pass, _ := u.User.Password()
	advancedConfig := &transmissionrpc.AdvancedConfig{}
	if u.Scheme == "https" {
		advancedConfig.HTTPS = true
		advancedConfig.Port = 443
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err == nil {
			advancedConfig.Port = uint16(port)
		}
	}
	transmissionbt, err := transmissionrpc.New(
		u.Hostname(),
		u.User.Username(),
		pass,
		advancedConfig,
	)
	if err != nil {
		return nil, err
	}
	return transmissionbt, err
}

func main() {
	db, err := bolt.Open("./db", 0664, nil)
	check(err)

	cfg := &Config{
		Connection: "https://adam:test@tr.adambibby.ca",
		Series: []*Series{
			{
				Title: "Bofuri",
				RSS:   "https://nyaa.si/?page=rss&q=%5BJudas%5D+Itai+No+Wa+Iya+Nano+De+Bougyoryoku+Ni+Kyokufuri+Shitai+To+Omoimasu+%28Bofuri%29+1080&c=0_0&f=0",
			},
			{
				Title:  "Somali to Mori no Kamisama",
				RSS:    "http://www.horriblesubs.info/rss.php?res=1080",
				RegExp: "Somali to Mori no Kamisama",
			},
		},
	}

	client, err := btClient(cfg.Connection)
	check(err)

	for _, series := range cfg.Series {
		check(download(db, client, series))
	}
}

func initBucket(db *bolt.DB, name []byte) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			_, err := tx.CreateBucket(name)
			if err != nil {
				return fmt.Errorf("create bucket: %s", err)
			}
		}
		return nil
	})
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

		id := []byte(item.Link)

		err = db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(bucket)
			g := b.Get(id)
			if g == nil {
				log.Printf("Downloading %s", item.Title)

				torrent, err := transmissionbt.TorrentAdd(&transmissionrpc.TorrentAddPayload{
					Filename: &item.Link,
				})
				check(err)

				d := &Download{
					ID:               id,
					TorrentURL:       item.Link,
					Downloading:      true,
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
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}
