/*

App Server for GoLab to server web browsers

Usage:
go run main.go 
*/

package main

import (
	"net/http"
	"fmt"
	"encoding/json"
)

type SessionSettings struct {
	workerIP 	string `json: workerIP`
	sessID 		string `json: sessID`
}

func main() {
	PORT := ":8080"
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

		var sessionSettings SessionSettings
		sessionSettings.sessID = sessID

		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		json.NewEncoder(w).Encode(sessionSettings)
		if newPage == "true" {
			// Serves the webpage 
			http.ServeFile(w, r, "./public/playground.html")
		}
	}
}

