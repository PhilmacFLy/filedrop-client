package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"golang.org/x/crypto/sha3"
)

var server string
var username string
var password string
var del bool
var filename string
var hash bool

type userconfig struct {
	Server   string
	Username string
	Password string
}

type uploadResponse struct {
	URL     string
	Expires time.Time
}

var conf userconfig

func (u *userconfig) load(path string) error {
	body, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, &u)
	return err
}

func (u *userconfig) save(path string) error {
	j, err := json.MarshalIndent(&u, "", "    ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, j, 0644)
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func createhash(input string) string {
	buf := []byte("some data to hash")
	// A hash needs to be 64 bytes long to have 256-bit collision resistance.
	h := make([]byte, 64)
	// Compute a 64-byte hash of buf and put it in h.
	sha3.ShakeSum256(h, buf)
	return hex.EncodeToString(h[:])
}

func main() {
	flag.StringVar(&server, "s", "", "Sets the serveradress to post to. Overrides values in config.")
	flag.StringVar(&username, "u", "", "Sets the username to use. Overrides values in config.")
	flag.StringVar(&password, "p", "", "Sets password to log in as. Must be supplied in cleartext. Overrides values in config.")
	flag.BoolVar(&del, "d", false, "Used to delete already uploaded files")
	flag.StringVar(&filename, "f", "", "Sets the name of the file after upload")
	flag.Parse()

	if len(flag.Args()) > 1 {
		log.Fatal("Usage: filedrop [-s server] [-u user] [-p password] [-d] [-h] [-f filename] filepath")
	}

	user, err := user.Current()
	if err != nil {
		log.Fatal("Error getting HomeDir:", err)
	}
	configpath := filepath.FromSlash(user.HomeDir + "/.config/filedrop/")
	cont, err := exists(configpath + "config.json")
	if err != nil {
		log.Fatal("Error checking configpath:", err)
	}
	if cont {
		err = conf.load(configpath + "config.json")
		if err != nil {
			log.Fatal("Error loading default config:", err)
		}
	}
	if (server == "") && (username == "") && (password == "") {
		if !cont {
			err = os.MkdirAll(configpath, 0755)
			if err != nil {
				log.Fatal("Error creating config folder:", err)
			}
			fmt.Println("Seems you haven't set any default values yet. Please set them now:")
			fmt.Println("Server Adress:")
			fmt.Scanln(&conf.Server)
			fmt.Println("Username:")
			fmt.Scanln(&conf.Username)
			fmt.Println("Password (cleartext):")
			fmt.Scanln(&conf.Password)
			conf.Password = createhash(conf.Password)
			err = conf.save(configpath + "config.json")
			if err != nil {
				log.Fatal("Error loading saving config:", err)
			}
		}
	}
	if server != "" {
		conf.Server = server
	}
	if username != "" {
		conf.Username = username
	}
	if password != "" {
		conf.Password = password
	}

	fp := flag.Args()[0]

	if !del {
		file, err := os.Open(fp)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", filepath.Base(fp))
		if err != nil {
			log.Fatal(err)
		}
		_, err = io.Copy(part, file)

		err = writer.Close()
		if err != nil {
			log.Fatal(err)
		}

		req, err := http.NewRequest("POST", conf.Server, body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		fn := base64.StdEncoding.EncodeToString([]byte(filepath.Base(fp)))
		xfn := ""
		if filename != "" {
			xfn = base64.StdEncoding.EncodeToString([]byte(filepath.Base(filename)))
		}
		req.Header.Set("Filename", fn)
		req.Header.Set("X-Filename", xfn)
		fmt.Println(conf.Password)
		req.SetBasicAuth(conf.Username, conf.Password)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}

		if resp.StatusCode != 200 {

			body := &bytes.Buffer{}
			_, err = body.ReadFrom(resp.Body)
			if err != nil {
				log.Fatal(err)
			}
			resp.Body.Close()
			log.Fatal("Serverside error", resp.StatusCode, body)
		}

		decoder := json.NewDecoder(resp.Body)
		var ur uploadResponse
		err = decoder.Decode(&ur)
		if err != nil {
			log.Fatal("Error decoding json:", err)
		}
		defer resp.Body.Close()

		fmt.Println("File successfully uploaded")
		fmt.Println("URL: ", ur.URL)
		fmt.Println("It expires: ", ur.Expires.Format(time.RFC3339))
	}
}
