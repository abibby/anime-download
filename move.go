package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"

	"github.com/abibby/transmissionrpc"
	"github.com/davecgh/go-spew/spew"
	bolt "go.etcd.io/bbolt"
)

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
					s := cfgSeries(cfg, d.Series)
					fileName := file.Name
					if s != nil && s.EpisodeRegExp != "" {
						re, err := regexp.Compile(s.EpisodeRegExp)
						if err != nil {
							log.Print(err)
						} else {
							groups := regexpGroups(re, file.Name)
							fileName = d.Series + " "
							if season, ok := groups["season"]; ok {
								fileName += fmt.Sprintf("S%s", season)
							}
							if episode, ok := groups["episode"]; ok {
								fileName += fmt.Sprintf("E%s", episode)
							}
							fileName += path.Ext(file.Name)
							spew.Dump(fileName)
						}
					}
					spew.Dump(fileName)
					os.Exit(1)
					dst := path.Join(cfg.CompletePath, d.Series, fileName)
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

func regexpGroups(re *regexp.Regexp, s string) map[string]string {
	matches := re.FindStringSubmatch(s)
	result := make(map[string]string)

	names := re.SubexpNames()
	for i, match := range matches {
		name := names[i]
		if i != 0 && name != "" {
			result[name] = match
		}
	}

	return result
}

func cfgSeries(cfg *Config, series string) *Series {
	for _, s := range cfg.Series {
		if s.Title == series {
			return s
		}
	}
	return nil
}
