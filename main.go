package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
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

func (t *Task) Check(cn chan Stat) {
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
	cn <- Stat{ID: t.ID, Status: tmpStatus}
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

func SendStat(s []Stat, host string) (errBool bool) {

	fjson := new(bytes.Buffer)
	errBool = CheckError(json.NewEncoder(fjson).Encode(s))
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

	cn := make(chan Stat, 10)					//channel length

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
				for ii := range tL {
					if tL[ii].ID == res.ID {
						if tL[ii].Status != res.Status {
							tL[ii].Status = res.Status
							statArr[tsec] = res
							tsec++
						}
					}
				}
			default:
				fmt.Printf("Channel is empty\n")
			}
		}
		if tsec != 0 { 						//check
			statArrTmp := make([]Stat, tsec)
			for jj := range statArrTmp {
				statArrTmp[jj] = statArr[jj]
			}	
			SendStat(statArrTmp, *host)
		}
		fmt.Printf("Stage №:%d - done\n", fsec + 1)		//debug

		tsec = 0
		ssec = 0

		if fsec != 59 {
			fsec++
		} else {
			tmptL := tL
			tL = GetTasks(*host, u.ID)
			fmt.Printf("Set old statuses\n")
			for iii := range tL {
				for jjj := range tmptL {
					if tmptL[jjj].ID == tL[iii].ID {
						tL[iii].Status = tmptL[jjj].Status
					}
				}
			}
			fsec = 0
		}
	}
}
