/*

App Server for GoLab to server web browsers

Usage:
go run main.go
*/

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/rpc"
)

type SessionSettings struct {
	WorkerIP string `json:"WorkerIP"`
	SessID   string `json:"SessID"`
}

var LBConn *rpc.Client

func main() {
	PORT := ":8080"

	// Getting load balancer IP from cmd line argument
	// args := os.Args[1:]
	// lbAddr := args[0]
	// LBConn, err := rpc.Dial("tcp", lbAddr)
	// if err != nil {
	// 	log.Fatalln("Couldn't connect to Load Balancer")
	// }

	http.Handle("/", http.FileServer(http.Dir("./public")))
	http.HandleFunc("/register", RegisterHandler)
	fmt.Println("Listening on: ", PORT)
	http.ListenAndServe(PORT, nil)
}

func RegisterHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			fmt.Println("Error Parsing Form: ", err)
			return
		}
		sessID := r.FormValue("sessID")
		newPage := r.FormValue("newPage")
		fmt.Println("Session ID: ", sessID)
		fmt.Println("New Page : ", newPage)

		// TODO: RPC Call to Load Balancer
		// 		set sessionSettings.workerIP with return address

		//LBConn.Call("LBServer.RegisterNewClient", request, response)

		sessionSettings := *new(SessionSettings)
		sessionSettings.SessID = sessID
		fmt.Println(sessionSettings)
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		json.NewEncoder(w).Encode(sessionSettings)
		if newPage == "true" {
			// Serves the webpage
			http.ServeFile(w, r, "./public/playground.html")
		}
	}
}
