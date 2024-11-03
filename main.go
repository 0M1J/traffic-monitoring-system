package main

import (
	"fmt"
	"net/http"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello, World!")
}

func main() {
	modelSession, err := initSession()
	if err != nil {
		fmt.Printf("Error creating session and tensors: %s\n", err)
	}
	defer modelSession.Destroy()

	http.HandleFunc("/", helloHandler)

	fmt.Println("Starting server on :8090")
	if err := http.ListenAndServe(":8090", nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}
