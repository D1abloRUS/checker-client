package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net"
	"time"
	"os"
	"log"
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
	log.Printf("Error: %s\n", err)
	return true
}

func (c *connection) Conn() (net.Conn, bool) {
	conn, err := net.DialTimeout(c.protocol, c.address, 250*time.Millisecond)
	return conn, CheckError(err)
}

func (t *Task) Check(cn chan Stat) {
	var (
		tmpStatus bool
	)

	c := connection{
		protocol: "tcp", // hardcode because http(s)
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
	log.Printf("Check: ID:%d, Status:%t\n", t.ID, tmpStatus) //debug
	cn <- Stat{ID: t.ID, Status: tmpStatus}
}

func (c *Client) Activate(host string) {
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(c)
	CheckError(err)
	log.Printf("Json send: %s", b)			//debug, do not use \n with json

	//Тут хардкод url api
	res, err := http.Post(fmt.Sprintf("https://%s/api/v1/activate", host), "application/json; charset=utf-8", b)
	if err == nil {
		defer res.Body.Close()
	} else {
		CheckError(err)
		os.Exit(1)
	}

	err = json.NewDecoder(res.Body).Decode(&c)
	CheckError(err)
	json.NewEncoder(b).Encode(c)			//debug
	log.Printf("Json recive: %s", b)		//debug, do not use \n with json
}

func GetTasks(host string, id int) []Task {
	var t []Task

	r, err := http.Get(fmt.Sprintf("https://%s/api/v1/gettask/%d", host, id))
	CheckError(err)

	defer r.Body.Close()

	res, err := ioutil.ReadAll(r.Body)
	CheckError(err)

	log.Printf("Getting task: %s", res)		//debug, do not use \n with json

	err = json.Unmarshal(res, &t)
	CheckError(err)

	return t
}

func SendStat(s []Stat, host string) {
	bjson := new(bytes.Buffer)
	err := json.NewEncoder(bjson).Encode(s)
	CheckError(err)
	log.Printf("Status json sending: %v", bjson)	//debug, do not use \n with json

	res, err := http.Post(fmt.Sprintf("https://%s/api/v1/statusupdate", host), "application/json; charset=utf-8", bjson)
	if err == nil {
		defer res.Body.Close()
		log.Printf("Status json is send\n")	//debug
	} else {
		CheckError(err)
		os.Exit(1)
	}
}

func main() {
	var (
		help = flag.Bool("help", false, "use -help to see this information")
		host = flag.String("s", "", "input check server dns-name or address")
		hash = flag.String("h", "", "input user hash id")
		fsec int 			//main loop timer
		ssec int 			//gorutine counter
		tsec int 			//answer counter
		taskList []Task
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
	taskList = GetTasks(*host, u.ID)				//GetTask

	cn := make(chan Stat, 10)					//channel length

	for {								//eternal main loop 
		for i := range taskList {				//loop for start gorutine
			if (fsec % taskList[i].Interval) == 0 { 	//division without a remainder to find time of check
				ssec++					//gorutine counter
				go taskList[i].Check(cn)		//Check host
			}
		}

		time.Sleep(time.Second) 				//pause for closing gorutines

		statArr := make([]Stat, ssec)

		for j := 0; j < ssec; j++ { 				//read the channel 
			select {
			case res := <-cn:
				for k := range taskList {
					if taskList[k].ID == res.ID {	//check normal answer
						if taskList[k].Status != res.Status {
							taskList[k].Status = res.Status
							statArr[tsec] = res
							tsec++
						}
					}
				}
			default:
				log.Printf("Channel is empty\n")	//debug
			}
		}
		if tsec != 0 { 						//check avalible answers
			statArrTmp := make([]Stat, tsec)
			for l := range statArrTmp {
				statArrTmp[l] = statArr[l]
			}	
			SendStat(statArrTmp, *host)
		}
		log.Printf("Stage №:%d - done\n", fsec + 1)		//debug

		tsec = 0
		ssec = 0

		if fsec != 59 {						//check main loop counter for restart loop
			fsec++
		} else {
			tmpTaskList := taskList
			taskList = GetTasks(*host, u.ID)
			log.Printf("Set previous statuses\n")		//debug
			for f := range taskList {
				for h := range tmpTaskList {
					if tmpTaskList[h].ID == taskList[f].ID {
						taskList[f].Status = tmpTaskList[h].Status
					}
				}
			}
			fsec = 0
		}
	}
}
