package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	//	"log"
	//	"io"
	"os"
)

type connection struct {
	protocol string
	address  string
}

type Client struct {
	Hash string `json:"hash"`
	ID   int    `json:"id"`
}

type Task struct {
	ID       int    `json:"id"`
	Target   string `json:"target"`
	Interval int    `json:"interval"`
	Status   bool   `json:"status"`
}

type Stat struct {
	ID     int  `json:"id"`
	Status bool `json:"status"`
}

func CheckError(err error) bool {
	if err == nil {
		return false
	}
	fmt.Printf("error: %s\n", err)
	return true
}

func (c *connection) Conn() (net.Conn, bool) {
	conn, err := net.DialTimeout(c.protocol, c.address, 250*time.Millisecond)
	errBool := CheckError(err)
	return conn, errBool
}

func (t *Task) Check(cn chan string) {
	//func (t *Task) check(cn chan []byte) {
	var (
		tmpStatus string
	)
	c := connection{
		protocol: "tcp", // захардкожено потому что http
		address:  t.Target,
	}

	conn, err := c.Conn()
	if err == false {
		conn.Close()
		tmpStatus = true
		if t.Status != tmpStatus {
			t.Status = tmpStatus
		} else if t.Status == tmpStatus {
			tmpStatus = "no-change"
		}
	} else {
		time.Sleep(250 * time.Millisecond)
		conn, errr := c.Conn()
		if errr != false {
			tmpStatus = false
			if t.Status != tmpStatus {
				t.Status = tmpStatus
			} else if t.Status == tmpStatus {
				tmpStatus = "no-change"
			}
		} else {
			conn.Close()
		}
	}

	cn <- fmt.Sprintf("%d %s", t.ID, t.Status)
}

func (c *Client) Activate(host string) bool {
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(c)
	errBool := CheckError(err)
	//	fmt.Printf("json in epta: %s\n", b)		//debugg

	//Тут хардкод url api
	res, err := http.Post(fmt.Sprintf("https://%s/api/v1/activate", host), "application/json; charset=utf-8", b)
	errBool = CheckError(err)
	if err == nil {
		defer res.Body.Close()
	} else {
		os.Exit(1)
	}

	err = json.NewDecoder(res.Body).Decode(&c)
	errBool = CheckError(err)
	//	json.NewEncoder(b).Encode(c)			//debug
	//	fmt.Printf("json out epta: %s\n", b)		//debug
	return errBool
}

func GetTasks(host string, id int) []Task {
	var t []Task

	r, err := http.Get(fmt.Sprintf("https://%s/api/v1/gettask/%d", host, id))
	CheckError(err)

	defer r.Body.Close()

	res, err := ioutil.ReadAll(r.Body)
	CheckError(err)

	//	fmt.Printf("json in epta: %s\n", res)		//debug

	err = json.Unmarshal(res, &t)
	CheckError(err)

	//	for i := range t {				//debug
	//		fmt.Printf("%d :%d : %d : %s : %t\n", i, t[i].ID, t[i].Interval, t[i].Target, t[i].Status)
	//	}

	return t
}

func SetStat(info string) Stat {

	str := strings.Split(info, (" "))

	id, err := strconv.Atoi(str[0])
	CheckError(err)

	st, err := strconv.ParseBool(str[1])
	CheckError(err)
	s := Stat{ID: id, Status: st}

	//	fmt.Printf("json out epta: %s\n", b)		//debug

	return s
}

func SendStat(s []Stat, host string) bool {
	fjson := new(bytes.Buffer)
	err := json.NewEncoder(fjson).Encode(s)
	errBool := CheckError(err)
	//	fmt.Printf("json for send epta: %s\n", fjson)	//debug

	res, err := http.Post(fmt.Sprintf("https://%s/api/v1/statusupdate", host), "application/json; charset=utf-8", fjson)
	errBool = CheckError(err)
	if err == nil {
		defer res.Body.Close()
		//		fmt.Printf("Status is push\n")		//debug
	} else {
		os.Exit(1)
	}

	return errBool
}

func main() {
	var (
		help = flag.Bool("help", false, "use -help to see this information")
		host = flag.String("s", "", "input check server dns-name or address")
		hash = flag.String("h", "", "input user hash id")
		fsec int //main loop timer
		ssec int //task counter
	)
	flag.Parse()

	if len(os.Args) == 1 {
		flag.PrintDefaults()
		os.Exit(1)
	} else if *help == true {
		flag.PrintDefaults()
		os.Exit(0)
	}

	u := Client{Hash: *hash}
	u.Activate(*host)           //Activate - return 0 as success
	tL := GetTasks(*host, u.ID) //GetTask

	//	for i := range tL {				//debug
	//		fmt.Printf("Id:%d  Interval:%d  Target:%s  Status:%t\n", tL[i].ID, tL[i].Interval, tL[i].Target, tL[i].Status)
	//	}

	cn := make(chan string, 10) //максимальная очередь задач

	for { //основнной цикл должен быть бесконечным

		for j := range tL { //цикл запуска горутин

			if (fsec % tL[j].Interval) == 0 { //проверка интервала, если делится без остатка, то время пришло

				//				fmt.Printf("Checking: %s - intr: %d\n", tL[j].Target, tL[j].Interval) //debug
				ssec++
				go tL[j].Check(cn)
			}
		}

		time.Sleep(time.Second)

		fstat := make([]Stat, ssec)

		for j := 0; j < ssec; j++ { // сбор статусов по задачам
			select {
			case res := <-cn:
				//				fmt.Printf("Result: %s\n", res) 	//debug
				fstat[j] = SetStat(res) //подготовка
			default:
				//				fmt.Printf("Channel is empty\n")	//debug
			}
		}
		//sendStat
		if ssec != 0 { //если что либо проверялось то отправляем результат
			SendStat(fstat, *host)
		}

		//		fmt.Printf("Проход №:%d выполнен!\n", fsec + 1)		//debug
		ssec = 0
		if fsec != 59 {
			fsec++
		} else {
			tL = GetTasks(*host, u.ID)
			fsec = 0
		}
	}
}
