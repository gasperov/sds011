package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kabukky/httpscerts"
	"github.com/tarm/serial"
)

//go:generate go run scripts/includetxt.go static/index.html static/arduino.ino static/arduino.jpg main.go sds011.go

func p(err error) {
	if err != nil {
		panic(err)
	}
}

type values struct {
	Last    string    `json:"last"`
	Started int64     `json:"started"`
	Pm25    []float64 `json:"pm25"`
	Pm10    []float64 `json:"pm10"`
	Ts      []int64   `json:"ts"`
}

func getTs() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

type decoder struct {
	buf    []byte
	start  int64
	lastTs int64
	rxPm   *regexp.Regexp
	rxTs   *regexp.Regexp
	out    *os.File

	mutex *sync.Mutex

	lastMsg string
	pm25    []float64
	pm10    []float64
	ts      []int64
}

func NewDecoder() decoder {
	ret := decoder{}
	ret.rxPm = regexp.MustCompile(`ug/m3 PM2\.5=(\d+\.\d+), PM10=(\d+\.\d+)`)
	ret.rxTs = regexp.MustCompile(`ts=(\d+)`)
	ret.mutex = &sync.Mutex{}

	{
		files, err := ioutil.ReadDir(".")
		p(err)
		for _, file := range files {
			ret.addFile(file.Name())
		}
	}

	now := getTs()
	if ret.start == 0 {
		ret.start = now
	}

	fn := fmt.Sprintf("sds011_%v.dat", now)
	var err error
	ret.out, err = os.Create(fn)
	p(err)
	return ret
}
func (d *decoder) get() values {
	n := getTs()
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return values{d.lastMsg, n, d.pm25, d.pm10, d.ts}
}
func (d *decoder) addFile(f string) {
	fmt.Printf("checking %v\n", f)
	re := regexp.MustCompile(`sds011_(\d+)\.dat`)
	res := re.FindAllStringSubmatch(f, -1)
	if len(res) != 1 {
		return
	}
	ts, err := strconv.ParseInt(res[0][1], 10, 64)
	if err != nil {
		return
	}

	if d.lastTs > ts {
		fmt.Printf("invalid timestamps %v ... %v\n", d.lastTs, ts)
		return
	}
	fmt.Printf("reading ...\n")
	d.lastTs = ts
	if d.start == 0 {
		d.start = ts
	}
	data, err := ioutil.ReadFile(f)
	p(err)
	d.add(data)
}
func (d *decoder) nextLine() string {
	for {
		pos := bytes.IndexByte(d.buf, 10) // find new line \n
		if pos == -1 {
			if len(d.buf) > 1024 {
				d.buf = append([]byte(nil), d.buf[64:]...)
			}
			return ""
		}

		b := append([]byte(nil), d.buf[pos+1:]...)
		if pos > 0 {
			s := string(d.buf[:pos])
			d.buf = b
			return s
		}
		d.buf = b
	}
}
func (d *decoder) add(b []byte) {
	d.buf = append(d.buf, b...)

	for {
		s := d.nextLine()
		if s == "" {
			return
		}

		if d.out == nil {
			res := d.rxTs.FindAllStringSubmatch(s, -1)
			for i := range res {
				ts, err := strconv.ParseInt(res[i][1], 10, 64)
				if err != nil {
					continue
				}
				if d.lastTs > ts {
					panic(fmt.Sprintf("invalid timestamps %v ... %v\n", d.lastTs, ts))
				}
				d.lastTs = ts
			}
		}
		{
			res := d.rxPm.FindAllStringSubmatch(s, -1)
			for i := range res {
				fmt.Printf("%v - %s, %s\n", res[i][0], res[i][1], res[i][2])

				fa, err := strconv.ParseFloat(res[i][1], 64)
				if err != nil {
					fmt.Printf("invalid input: %s\n", res[i][1])
					continue
				}
				fb, err := strconv.ParseFloat(res[i][2], 64)
				if err != nil {
					fmt.Printf("invalid input: %s\n", res[i][2])
					continue
				}

				var ts int64
				if d.out == nil {
					ts = d.lastTs
					d.lastTs += 990 // assume arduino/sensor will dump 1/s
				} else {
					ts = getTs()
					if (ts - d.lastTs) > 60000 {
						d.lastTs = ts
						d.out.Write([]byte(fmt.Sprintf("ts=%v\n", ts)))
					}
					d.out.Write([]byte(fmt.Sprintf("%v\n", res[i][0])))
				}
				//fmt.Printf("%v\n", ts)

				func() {
					d.mutex.Lock()
					defer d.mutex.Unlock()
					d.lastMsg = res[i][0]
					d.pm25 = append(d.pm25, fa)
					d.pm10 = append(d.pm10, fb)
					d.ts = append(d.ts, ts-d.start)
					if len(d.pm25) > 30*24*60*60 {
						d.pm25 = append([]float64(nil), d.pm25[3600:]...)
						d.pm10 = append([]float64(nil), d.pm10[3600:]...)
						d.ts = append([]int64(nil), d.ts[3600:]...)
					}
				}()
			}
		}
	}
}

func main() {
	dec := NewDecoder()

	if len(os.Args) > 1 {
		fmt.Printf("opening port %v\n", os.Args[1])
		go func() {
			dely := 0
			if len(os.Args) > 2 {
				d, err := strconv.ParseInt(os.Args[2], 10, 32)
				p(err)
				dely = int(d)
			}

			sds := SDS011{}
			config := &serial.Config{
				Name:        os.Args[1], //"com3",
				Baud:        9600,       //57600,
				ReadTimeout: time.Minute * 30,
				Size:        8,
			}

			stream, err := serial.OpenPort(config)
			p(err)

			time.Sleep(4 * time.Second)
			first := true
			_, err = stream.Write(sds.SetPeriod(dely))
			p(err)
			buf := make([]byte, 1024)
			for {
				n, err := stream.Read(buf)
				p(err)
				if sds.ReadBytes(buf[:n]) {
					dec.add([]byte(fmt.Sprintf("ug/m3 PM2.5=%v, PM10=%v\n", sds.PM25, sds.PM10)))
					if first {
						first = false
						_, err = stream.Write(sds.SetPeriod(dely))
						p(err)
					}
				}
			}
		}()
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fn := filepath.Base(r.URL.Path)
		if fn == "data.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(dec.get())
			return
		}
		if fn == "files.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fn := []string{}
			for key := range static_files {
				fn = append(fn, key)
			}
			json.NewEncoder(w).Encode(fn)
			return
		}
		if fn == "static_files.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(static_files)
			return
		}
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fn = "index.html"
		} else if strings.HasSuffix(fn, ".jpg") {
			w.Header().Set("Content-Type", "image/jpeg")
		} else {
			w.Header().Set("Content-Type", "text/plain")
		}
		w.WriteHeader(http.StatusCreated)
		if val, ok := static_files[fn]; ok {
			data, err := hex.DecodeString(val)
			p(err)
			w.Write(data)
			return
		}
		fmt.Fprintf(w, "Hello world!")
	})

	err := httpscerts.Check("cert.pem", "key.pem")
	if err != nil {
		err = httpscerts.Generate("cert.pem", "key.pem", "127.0.0.1:8081")
		if err != nil {
			log.Fatal("Error: Couldn't create https certs.")
		}
	}
	/*go http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		target := "https://" + req.Host + req.URL.Path
		if len(req.URL.RawQuery) > 0 {
			target += "?" + req.URL.RawQuery
		}
		log.Printf("redirect to: %s", target)
		http.Redirect(w, req, target, http.StatusTemporaryRedirect)
	}))*/
	fmt.Printf("open https://localhost:8081/\n")
	http.ListenAndServeTLS(":8081", "cert.pem", "key.pem", nil)

}
