package main

import (
	"net/http"
	"fmt"
)

func main() {
	PORT := ":8080"
	http.Handle("/", http.FileServer(http.Dir("./public")))
	fmt.Println("Listening on: ", PORT)
	http.ListenAndServe(PORT, nil)
}

