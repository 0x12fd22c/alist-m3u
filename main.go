package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

var groups map[Folder][]string
var s sync.RWMutex

type Config struct {
	Folders []Folder `yaml:"folders"`
}

type Folder struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	Path string `yaml:"path"`
}

func main() {
	groups = map[Folder][]string{}
	data, err := ioutil.ReadFile("sub.yaml")
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return
	}
	var conf Config
	err = yaml.Unmarshal(data, &conf)
	if err != nil {
		return
	}

	wg := sync.WaitGroup{}
	wg.Add(len(conf.Folders))
	for _, folder := range conf.Folders {
		go func(folder Folder) {
			getDir(folder)
			wg.Done()
		}(folder)
	}
	wg.Wait()
	generate()
}

func generate() {
	for k, v := range groups {
		if len(v) == 0 {
			continue
		}

		var tracks []Track
		for _, c := range v {
			tracks = append(tracks, Track{
				Name:   filepath.Base(c),
				Length: -1,
				URI:    fmt.Sprintf("https://raw.popeyelau.repl.co/?r=%s%s", k.Host, c),
				Tags: []Tag{
					{
						Name:  "group-title",
						Value: k.Name,
					},
				},
			})
		}

		sort.Slice(tracks, func(i, j int) bool {
			return tracks[i].Name > tracks[j].Name
		})

		playlist := Playlist{Tracks: tracks}
		reader, err := Marshall(playlist)
		if err != nil {
			fmt.Println(err)
			return
		}
		b := reader.(*bytes.Buffer)
		_ = ioutil.WriteFile(fmt.Sprintf("%s.m3u", k.Name), b.Bytes(), os.ModePerm)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

}

func getDir(folder Folder) {
	client := &http.Client{}
	target := fmt.Sprintf("%s/api/fs/list", folder.Host)
	var jsonData = []byte(fmt.Sprintf(`{
		"path": "%s"
	}`, folder.Path))
	req, err := http.NewRequest("POST", target, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	result := gjson.GetBytes(respBody, "data.content")
	if result.IsArray() {
		folders := result.Array()
		for _, v := range folders {
			isDir := v.Get("is_dir").Bool()
			fileType := v.Get("type").Int()
			path := fmt.Sprintf("%s/%s", folder.Path, v.Get("name").String())
			if !isDir && fileType != 2 {
				continue
			}

			if isDir {
				continue
			}
			s.Lock()
			groups[folder] = append(groups[folder], path)
			s.Unlock()

		}
	}
}

// Playlist is a type that represents an m3u playlist containing 0 or more tracks
type Playlist struct {
	Tracks []Track
}

// A Tag is a simple key/value pair
type Tag struct {
	Name  string
	Value string
}

// Track represents an m3u track with a Name, Lengh, URI and a set of tags
type Track struct {
	Name   string
	Length int
	URI    string
	Tags   []Tag
}

// Marshall Playlist to an m3u file.
func Marshall(p Playlist) (io.Reader, error) {
	buf := new(bytes.Buffer)
	w := bufio.NewWriter(buf)
	if err := MarshallInto(p, w); err != nil {
		return nil, err
	}

	return buf, nil
}

// MarshallInto a *bufio.Writer a Playlist.
func MarshallInto(p Playlist, into *bufio.Writer) error {
	into.WriteString("#EXTM3U\n")
	for _, track := range p.Tracks {
		into.WriteString("#EXTINF:")
		into.WriteString(fmt.Sprintf("%d, ", track.Length))
		for i := range track.Tags {
			if i == len(track.Tags)-1 {
				into.WriteString(fmt.Sprintf("%s=%q", track.Tags[i].Name, track.Tags[i].Value))
				continue
			}
			into.WriteString(fmt.Sprintf("%s=%q ", track.Tags[i].Name, track.Tags[i].Value))
		}
		if len(track.Tags) > 0 {
			into.WriteString(", ")
		}

		into.WriteString(fmt.Sprintf("%s\n%s\n", track.Name, track.URI))
	}

	return into.Flush()
}
