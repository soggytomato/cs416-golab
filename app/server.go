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

// TODO: Get rid of Globals!

type AppServer struct {
	LBConn          *rpc.Client
	CurrentSessions []string
	AllUsernames    []string
	logger          *log.Logger
}

func main() {
	appserver := new(AppServer)
	appserver.logger = log.New(os.Stdout, "[AppServer] ", log.Lshortfile)
	// Getting load balancer IP from cmd line argument
	args := os.Args[1:]
	if len(args) != 1 {
		appserver.logger.Fatalln("Missing Args Usage: go run main.go [LBServer IP:Port]")
	}

	PORT := ":8080"
	lbAddr := args[0]
	lbConn, err := rpc.Dial("tcp", lbAddr)
	if err != nil {
		appserver.logger.Fatalln("Couldn't connect to Load Balancer")
	}
	appserver.LBConn = lbConn

	appserver.CurrentSessions = make([]string, 0)
	appserver.AllUsernames = make([]string, 0)

	http.Handle("/", http.FileServer(http.Dir("./public")))
	http.HandleFunc("/register", appserver.RegisterHandler)
	http.HandleFunc("/sessions", appserver.SessionHandler)
	http.HandleFunc("/usernames", appserver.UsernameHandler)
	appserver.logger.Println("Listening on: ", PORT)
	http.ListenAndServe(PORT, nil)
}

// Function to handle a new client connecting to either a new or existing session
// AppServer contacts Load Balancer for a new worker IP and returns a SessionSetting to the browser
// Assumptions:
//		- Browser will do the form validation
// 		- Browser will handle the errors e.g., if no worker IP was given by the Load Balancer
//
func (ap *AppServer) RegisterHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == "POST" {
		ap.logger.Println("Got a /register POST Request")
		err := r.ParseForm()
		if err != nil {
			ap.logger.Println("Error Parsing Form: ", err)
			return
		}
		ap.logger.Println("Form: ", r.Form)
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
			for _, recUsername := range ap.AllUsernames {
				if username == recUsername {
					newUser = false
				}
			}
			if newUser {
				ap.AllUsernames = append(ap.AllUsernames, username)
			}
		} else {
			username = r.FormValue("existingUser")
		}

		var retWorkerIP string
		err = ap.LBConn.Call("LBServer.RegisterNewClient", sessID, &retWorkerIP)
		if err != nil {
			ap.logger.Println(err)
		}
		sessionSettings := *new(SessionSettings)
		sessionSettings.SessID = sessID
		sessionSettings.WorkerIP = retWorkerIP
		ap.logger.Println("Session Settings: ", sessionSettings)
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		json.NewEncoder(w).Encode(sessionSettings)
		newSession := true
		for _, sess := range ap.CurrentSessions {
			if sess == sessID {
				newSession = false
			}
		}
		if newSession {
			ap.CurrentSessions = append(ap.CurrentSessions, sessID)
		}

	}
}

// Function to return current sessions to the browser for the user to choose from
func (ap *AppServer) SessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		sessions := *new(Sessions)
		sessions.ExistingSessions = ap.CurrentSessions
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		json.NewEncoder(w).Encode(sessions)
	}
}

// Function to return any active or inactive username for the user to choose from
func (ap *AppServer) UsernameHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		users := *new(Users)
		users.AllUsernames = ap.AllUsernames
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		json.NewEncoder(w).Encode(users)
	}
}
