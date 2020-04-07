package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"

	"github.com/hekmon/transmissionrpc"
	"github.com/mmcdole/gofeed"
	bolt "go.etcd.io/bbolt"
	"gopkg.in/yaml.v2"
)

type Download struct {
	ID               []byte
	TorrentURL       string
	Compleated       bool
	Series           string
	TransmissionHash string
}

type Series struct {
	Title  string `yaml:"title"`
	RSS    string `yaml:"rss"`
	RegExp string `yaml:"regexp"`
}

type Config struct {
	Connection   string    `yaml:"connection"`
	Ratio        int       `yaml:"ratio"`
	Series       []*Series `yaml:"series"`
	DownloadPath string    `yaml:"download_path"`
	CompletePath string    `yaml:"complete_path"`
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
	if len(os.Args) < 2 {
		fmt.Print("anime-download <config path>\n")
		return
	}
	db, err := bolt.Open("./db", 0664, nil)
	check(err)

	b, err := ioutil.ReadFile(os.Args[1])
	check(err)

	cfg := &Config{}
	check(yaml.Unmarshal(b, cfg))

	client, err := btClient(cfg.Connection)
	check(err)

	for _, series := range cfg.Series {
		check(download(db, client, series))
	}

	check(move(db, client, cfg))

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

				file := ""
				if len(item.Enclosures) > 0 {
					file = item.Enclosures[0].URL
				} else if item.Link != "" {
					file = item.Link
				}
				torrent, err := transmissionbt.TorrentAdd(&transmissionrpc.TorrentAddPayload{
					Filename: &file,
				})
				check(err)

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
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func move(db *bolt.DB, client *transmissionrpc.Client, cfg *Config) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		return b.ForEach(func(k, v []byte) error {
			d := &Download{}
			err := json.Unmarshal(v, d)
			if err != nil {
				return err
			}

			if d.Compleated == true {
				return nil
			}

			torrents, err := client.TorrentGetAllForHashes([]string{d.TransmissionHash})
			if err != nil {
				return err
			}
			if len(torrents) == 0 {
				return nil
			}
			torrent := torrents[0]

			if torrent.DoneDate != nil && !torrent.DoneDate.IsZero() {

				log.Printf("Finished downloading %s", *torrent.Name)

				for _, file := range torrent.Files {
					dst := path.Join(cfg.CompletePath, d.Series, file.Name)
					err := os.MkdirAll(path.Dir(dst), 0755)
					if err != nil {
						return err
					}
					err = copyFile(path.Join(cfg.DownloadPath, file.Name), dst)
					if err != nil {
						return err
					}
				}

				d.Compleated = true
				by, err := json.Marshal(d)
				if err != nil {
					return err
				}
				err = b.Put(k, by)
				if err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func copyFile(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	return err
}
