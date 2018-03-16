/*

App Server for GoLab to server web browsers

Usage:
go run main.go [LBServer IP:Port]
*/

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/rpc"
	"os"
)

type SessionSettings struct {
	WorkerIP string `json:"WorkerIP"`
	SessID   string `json:"SessID"`
}

type Sessions struct {
	ExistingSessions []string `json:"ExistingSessions"`
}

type Users struct {
	AllUsernames []string `json:"AllUsernames"`
}

var LBConn *rpc.Client
var CurrentSessions []string
var AllUsernames []string
var logger *log.Logger

func main() {
	logger = log.New(os.Stdout, "[AppServer] ", log.Lshortfile)
	// Getting load balancer IP from cmd line argument
	args := os.Args[1:]
	if len(args) != 1 {
		logger.Fatalln("Missing Args Usage: go run main.go [LBServer IP:Port]")
	}

	PORT := ":8080"
	lbAddr := args[0]
	lbConn, err := rpc.Dial("tcp", lbAddr)
	if err != nil {
		logger.Fatalln("Couldn't connect to Load Balancer")
	}
	LBConn = lbConn

	CurrentSessions = make([]string, 0)
	AllUsernames = make([]string, 0)

	http.Handle("/", http.FileServer(http.Dir("./public")))
	http.HandleFunc("/register", RegisterHandler)
	http.HandleFunc("/sessions", SessionHandler)
	http.HandleFunc("/usernames", UsernameHandler)
	logger.Println("Listening on: ", PORT)
	http.ListenAndServe(PORT, nil)
}

// Function to handle a new client connecting to either a new or existing session
// AppServer contacts Load Balancer for a new worker IP and returns a SessionSetting to the browser
// Assumptions:
//		- Browser will do the form validation
// 		- Browser will handle the errors e.g., if no worker IP was given by the Load Balancer
//
func RegisterHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == "POST" {
		logger.Println("Got a /register POST Request")
		err := r.ParseForm()
		if err != nil {
			logger.Println("Error Parsing Form: ", err)
			return
		}
		logger.Println("Form: ", r.Form)
		var sessID string
		sessRadio := r.FormValue("sessionRadio")
		userRadio := r.FormValue("userRadio")
		if sessRadio == "existing" {
			sessID = r.FormValue("existingSession")
		} else {
			sessID = r.FormValue("session")
		}
		newUser := true
		var username string
		if userRadio == "new" {
			username = r.FormValue("newUser")
			for _, recUsername := range AllUsernames {
				if username == recUsername {
					newUser = false
				}
			}
			if newUser {
				AllUsernames = append(AllUsernames, username)
			}
		} else {
			username = r.FormValue("existingUser")
		}

		var retWorkerIP string
		err = LBConn.Call("LBServer.RegisterNewClient", sessID, &retWorkerIP)
		if err != nil {
			logger.Println(err)
		}
		sessionSettings := *new(SessionSettings)
		sessionSettings.SessID = sessID
		sessionSettings.WorkerIP = retWorkerIP
		logger.Println("Session Settings: ", sessionSettings)
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		json.NewEncoder(w).Encode(sessionSettings)
		newSession := true
		for _, sess := range CurrentSessions {
			if sess == sessID {
				newSession = false
			}
		}
		if newSession {
			CurrentSessions = append(CurrentSessions, sessID)
		}

	}
}

// Function to return current sessions to the browser for the user to choose from
func SessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		sessions := *new(Sessions)
		sessions.ExistingSessions = CurrentSessions
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		json.NewEncoder(w).Encode(sessions)
	}
}

// Function to return any active or inactive username for the user to choose from
func UsernameHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		users := *new(Users)
		users.AllUsernames = AllUsernames
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		json.NewEncoder(w).Encode(users)
	}
}
