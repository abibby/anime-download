package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	. "github.com/abibby/anime-download/try"
	"github.com/abibby/transmissionrpc"
	"go.etcd.io/bbolt"
	bolt "go.etcd.io/bbolt"
	"gopkg.in/yaml.v2"
)

type Series struct {
	Title         string `yaml:"title"`
	RSS           string `yaml:"rss"`
	RegExp        string `yaml:"regexp"`
	EpisodeRegExp string `yaml:"episode_regexp"`
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
	if err != nil {
		return nil, err
	}

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
	defer Handle(func(err error) {
		log.Fatal(err)
	})
	if len(os.Args) < 2 {
		fmt.Print("anime-download <config path>\n")
		return
	}
	db := Try(bbolt.Open("./db", 0664, nil))
	b := Try(ioutil.ReadFile(os.Args[1]))

	cfg := &Config{}
	Try0(yaml.Unmarshal(b, cfg))

	client := Try(btClient(cfg.Connection))

	for _, series := range cfg.Series {
		err := download(db, client, series)
		if err != nil {
			log.Print(err)
		}
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

// DownloadFile will download a url to a local file. It's efficient because it will
// write as it downloads and not load the whole file into memory.
func downloadFile(filepath string, url string) error {

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}
