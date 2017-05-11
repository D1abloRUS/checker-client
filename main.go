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
	var (
		tmpStatus bool
	)

	c := connection{
		protocol: "tcp", // захардкожено потому что http(s)
		address:  t.Target,
	}

	conn, err := c.Conn()
	if err == false { //no errors
		conn.Close()
		tmpStatus = true
	} else {
		time.Sleep(250 * time.Millisecond) //for wrong falure check
		conn, err := c.Conn()
		if err != false {
			tmpStatus = false
		} else {
			conn.Close()
		}
	}
	fmt.Printf("Check: ID:%d, Status:%t\n", t.ID, tmpStatus)
	cn <- fmt.Sprintf("%d %t", t.ID, tmpStatus)
}

func (c *Client) Activate(host string) bool {
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(c)
	errBool := CheckError(err)
	fmt.Printf("json in epta: %s\n", b)		//debugg

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
	json.NewEncoder(b).Encode(c)			//debug
	fmt.Printf("json out epta: %s\n", b)		//debug
	return errBool
}

func GetTasks(host string, id int) []Task {
	var t []Task

	r, err := http.Get(fmt.Sprintf("https://%s/api/v1/gettask/%d", host, id))
	CheckError(err)

	defer r.Body.Close()

	res, err := ioutil.ReadAll(r.Body)
	CheckError(err)

	fmt.Printf("Getting task: %s\n", res)		//debug

	err = json.Unmarshal(res, &t)
	CheckError(err)

	for i := range t {				//debug
		fmt.Printf("%d :%d : %d : %s : %t\n", i, t[i].ID, t[i].Interval, t[i].Target, t[i].Status)
	}

	return t
}

func SendStat(s []Stat, host string) bool {
//func SendStat(s map[int]Stat, host string) bool {
	fjson := new(bytes.Buffer)
	err := json.NewEncoder(fjson).Encode(s)
	errBool := CheckError(err)
	fmt.Printf("SendStat json: %v\n", fjson)	//debug

	res, err := http.Post(fmt.Sprintf("https://%s/api/v1/statusupdate", host), "application/json; charset=utf-8", fjson)
	errBool = CheckError(err)
	if err == nil {
		defer res.Body.Close()
		fmt.Printf("Status is push\n")		//debug
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
		fsec int 			//main loop timer
		ssec int 			//gorutine counter
		tsec int 			//answer counter
		tL	[]Task
	)

	flag.Parse()

	if len(os.Args) == 1 {
		flag.PrintDefaults()
		os.Exit(1)
	} else if *help == true {
		flag.PrintDefaults()
		os.Exit(0)
	}

	u := Client{Hash: *hash}					//set client hash
	u.Activate(*host)						//Activate
	tL = GetTasks(*host, u.ID)					//GetTask

	cn := make(chan string, 10)					//channel length

	for {								//eternal main loop 
		for i := range tL {					//loop for start gorutine
			if (fsec % tL[i].Interval) == 0 { 		//division without a remainder to find time of check
				ssec++					//gorutine counter
				go tL[i].Check(cn)			//Check
			}
		}

		time.Sleep(time.Second) 				//pause for closing gorutines

		statArr := make([]Stat, ssec)

		for j := 0; j < ssec; j++ { 				//read the channel 
			select {
			case res := <-cn:
				tmp_Str := strings.Split(res, (" "))
				tmp_Id, err := strconv.Atoi(tmp_Str[0])
				CheckError(err)
				tmp_St, err := strconv.ParseBool(tmp_Str[1])
				CheckError(err)
				for ii := range tL {
					if tL[ii].ID == tmp_Id {
						if tL[ii].Status != tmp_St {
							tL[ii].Status = tmp_St
							statArr[tsec] = Stat{ID: tmp_Id, Status: tmp_St}
							tsec++
						}
					}
				}
			default:
				fmt.Printf("Channel is empty\n")
			}
		}
		if tsec != 0 { 						//check 
			SendStat(statArr, *host)
		}
		fmt.Printf("Stage №:%d - done\n", fsec + 1)		//debug

		tsec = 0
		ssec = 0

		if fsec != 59 {
			fsec++
		} else {
			tmp_tL := tL
			tL = GetTasks(*host, u.ID)
			fmt.Printf("Set old statuses\n")
			for jji := range tL {
				for jj := range tmp_tL {
					if tmp_tL[jj].ID == tL[jji].ID {
						tL[jji].Status = tmp_tL[jj].Status
					}
				}
			}
			fsec = 0
		}
	}
}
